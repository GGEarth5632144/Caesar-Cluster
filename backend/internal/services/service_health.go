package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// service_health.go = ตัวที่ทำให้สถานะใน DB ตรงกับความจริงบนคลัสเตอร์
//
// "สร้าง object สำเร็จ" กับ "workload รันอยู่" เป็นคนละเรื่องกันบน k8s — API ตอบ 201 ทันที
// ส่วนการหา node ให้ Pod กับการที่ container จะขึ้นได้หรือไม่ เกิดทีหลังและล้มเหลวได้เงียบๆ
// ถ้าไม่มีตัวนี้ หน้าเว็บจะขึ้นจุดเขียวว่า running ทั้งที่ต่อเข้า service ไม่ได้เลย
//
// เป็น worker วนเช็คแทน goroutine ต่อการ deploy เพราะ (1) container ตายทีหลังได้ ไม่ใช่แค่ตอน
// เพิ่ง deploy (2) goroutine ต่อ request หายไปพร้อม process ตอน restart แล้วไม่มีใครตามต่อ
// (3) ตัวเดียวคุมความถี่ที่ยิงถามคลัสเตอร์ได้ ไม่ใช่ N ตัวต่างคนต่างยิง

// 10 วินาที = ผู้ใช้ยังรู้สึกว่าเกือบทันทีตอนกด deploy แล้วรอดูผล แต่ไม่ถี่จนเปลือง API ของคลัสเตอร์
const healthCheckInterval = 10 * time.Second

// service ที่นิ่งแล้วยังต้องถามซ้ำบ้าง ไม่งั้นตัวที่ตายทีหลังจะค้างเป็น running ตลอดไป
const settledRecheckAfter = 60 * time.Second

// เพดานต่อรอบ กันไม่ให้ยิงถามคลัสเตอร์เป็นร้อยครั้งรวด — ตัวที่เกินไปรอบถัดไป ไม่มีใครถูกลืม
// (เรียงจากตัวที่ไม่ได้เช็คนานสุดก่อน)
const maxServicesPerTick = 50

// ServiceHealthMonitor วนถามคลัสเตอร์แล้วอัปเดต services.status ให้ตรงกับของจริง
type ServiceHealthMonitor struct {
	db   *gorm.DB
	prov Provisioner
}

// NewServiceHealthMonitor ประกอบ monitor — ถูกเรียกจาก main ตอน start
func NewServiceHealthMonitor(db *gorm.DB, prov Provisioner) *ServiceHealthMonitor {
	return &ServiceHealthMonitor{db: db, prov: prov}
}

// Start เปิด worker เบื้องหลัง แล้วคืนทันที — หยุดเมื่อ ctx ถูก cancel (ตอนปิดเซิร์ฟเวอร์)
//
// เช็ครอบแรกทันทีโดยไม่รอ ticker ครบรอบ เพื่อให้ service ที่ค้างอยู่ตั้งแต่ก่อน restart
// ถูกจัดการทันทีที่ระบบกลับมา ไม่ใช่ค้างต่ออีก 10 วินาที
func (m *ServiceHealthMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()

		m.checkOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				log.Println("service health monitor: หยุดแล้ว")
				return
			case <-ticker.C:
				m.checkOnce(ctx)
			}
		}
	}()
	log.Printf("service health monitor: เริ่มทำงาน (ทุก %v)", healthCheckInterval)
}

// serviceWithNamespace = service หนึ่งตัวพร้อมชื่อ namespace บนคลัสเตอร์
// join มาในคำสั่งเดียวแทนที่จะ SELECT namespace ทีละตัว (กัน N+1 ในลูปที่วนทุก 10 วินาที)
type serviceWithNamespace struct {
	entity.Service
	NamespaceName string
}

// checkOnce เช็คหนึ่งรอบ — เลือกเฉพาะตัวที่ควรเช็คตอนนี้ แล้วอัปเดตตัวที่สถานะเปลี่ยน
func (m *ServiceHealthMonitor) checkOnce(ctx context.Context) {
	var rows []serviceWithNamespace

	// เงื่อนไขการเลือก: ตัวที่สถานะยังไม่นิ่ง (creating/pending/crashloop) เอาหมด
	// ส่วนตัวที่นิ่งแล้วเอาเฉพาะที่ไม่ได้เช็คมานานเกิน settledRecheckAfter
	//
	// เรียงตาม status_checked_at โดยให้ NULL (ไม่เคยเช็คเลย) มาก่อน — ตัวที่เพิ่ง deploy
	// คือตัวที่ผู้ใช้กำลังนั่งดูหน้าจอรออยู่ ต้องได้คิวก่อนเสมอ
	cutoff := time.Now().UTC().Add(-settledRecheckAfter)
	err := m.db.WithContext(ctx).
		Table("services AS s").
		Select("s.*, n.name AS namespace_name").
		Joins("JOIN namespaces n ON n.id = s.namespace_id").
		Where(`s.status IN ? OR s.status_checked_at IS NULL OR s.status_checked_at < ?`,
			[]string{entity.ServiceCreating, entity.ServicePending, entity.ServiceCrashLoop},
			cutoff).
		Order("s.status_checked_at ASC NULLS FIRST").
		Limit(maxServicesPerTick).
		Scan(&rows).Error
	if err != nil {
		log.Printf("service health monitor: อ่านรายการ service ไม่สำเร็จ: %v", err)
		return
	}

	// นับตัวที่ถามไม่ได้แล้วสรุปทีเดียวตอนจบรอบ แทนที่จะ log ทีละตัว
	//
	// จำเป็นเพราะการถามไม่สำเร็จจะไม่อัปเดต status_checked_at (เวลานั้นแปลว่า "ถามสำเร็จล่าสุด"
	// เมื่อไหร่) แถวนั้นจึงเข้าเงื่อนไขให้ถามใหม่ทุกรอบ ถ้า log ทีละตัว คลัสเตอร์ล่มตอนมี 50 service
	// = 300 บรรทัดต่อนาทีของข้อความเดียวกัน ซึ่งกลบ log อื่นที่สำคัญกว่าจนหมด
	var failed int
	var firstErr error
	var firstName string

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return // ปิดเซิร์ฟเวอร์ระหว่างวนอยู่ — หยุดเลย ไม่ต้องเช็คให้ครบ
		default:
		}
		if err := m.checkService(ctx, row); err != nil {
			failed++
			if firstErr == nil {
				firstErr, firstName = err, row.Name
			}
		}
	}

	if failed > 0 {
		suffix := ""
		if failed > 1 {
			suffix = fmt.Sprintf(" (และอีก %d ตัวในรอบนี้ด้วยเหตุผลเดียวกัน)", failed-1)
		}
		log.Printf("service health monitor: ถามสถานะ '%s' ไม่ได้%s: %v — คงสถานะเดิมไว้ก่อน",
			firstName, suffix, firstErr)
	}
}

// checkService ถามคลัสเตอร์เรื่อง service ตัวเดียวแล้วเขียนผลลง DB
// คืน error เฉพาะกรณี "ถามคลัสเตอร์ไม่ได้" เพื่อให้ผู้เรียกสรุปรวมทีเดียว (ดู checkOnce)
// ส่วนความผิดพลาดตอนเขียน DB จัดการ+log ในนี้เลย เพราะเป็นคนละเรื่องกับคลัสเตอร์ล่ม
func (m *ServiceHealthMonitor) checkService(ctx context.Context, row serviceWithNamespace) error {
	svc := row.Service

	status, err := m.prov.Status(ctx, row.NamespaceName, &svc)
	if err != nil {
		// ถามไม่ได้ ≠ ของพัง — คลัสเตอร์อาจล่มชั่วคราวหรือเน็ตมีปัญหา
		// ห้ามเขียน failed ลงไปเด็ดขาด ไม่งั้น service ที่รันอยู่ดีๆ จะกลายเป็นพังทั้งกระดาน
		// ตอนคลัสเตอร์สะดุดแค่แป๊บเดียว ปล่อยสถานะเดิมไว้แล้วลองใหม่รอบหน้า
		return err
	}

	newStatus, reason, message := translate(status)

	// PhaseGone (ของไม่อยู่บนคลัสเตอร์แล้ว) จะถูกเขียนเป็น failed ไม่ใช่ลบแถวทิ้ง — ลบเองแล้ว
	// ผู้ใช้จะเห็น service หายไปเฉยๆ โดยไม่รู้ว่าเกิดอะไรขึ้น และไม่รู้ว่าดิสก์ที่จองไว้ยังอยู่ไหม

	// ไม่มีอะไรเปลี่ยน — อัปเดตแค่เวลาที่เช็คล่าสุด ไม่ต้องเขียนทั้งแถว
	if newStatus == svc.Status && reason == svc.StatusReason &&
		status.Restarts == svc.RestartCount {
		m.touch(ctx, svc.ID)
		return nil
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"status":            newStatus,
		"status_reason":     reason,
		"status_message":    message,
		"restart_count":     status.Restarts,
		"status_checked_at": now,
	}

	// WithoutCancel: ถ้าเซิร์ฟเวอร์กำลังปิดระหว่างนี้ ผลที่ถามมาได้แล้วควรถูกบันทึก
	// ไม่ใช่ทิ้งไปแล้วปล่อยให้สถานะค้างผิดไว้จนกว่าจะ start ใหม่
	res := m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).
		Where("id = ?", svc.ID).Updates(updates)
	if res.Error != nil {
		log.Printf("service health monitor: อัปเดตสถานะ service id=%d ไม่สำเร็จ: %v", svc.ID, res.Error)
		return nil
	}
	if res.RowsAffected == 0 {
		return nil // ถูกลบไปแล้วระหว่างที่เรากำลังถามคลัสเตอร์ ไม่ใช่ปัญหา
	}

	if svc.Status != newStatus {
		log.Printf("service '%s' (id=%d): %s → %s%s",
			svc.Name, svc.ID, svc.Status, newStatus, reasonSuffix(reason))
	}
	return nil
}

// touch อัปเดตแค่เวลาที่เช็คล่าสุด สำหรับกรณีที่สถานะไม่เปลี่ยน
func (m *ServiceHealthMonitor) touch(ctx context.Context, serviceID int) {
	err := m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).
		Where("id = ?", serviceID).
		Update("status_checked_at", time.Now().UTC()).Error
	if err != nil {
		log.Printf("service health monitor: อัปเดตเวลาเช็ค service id=%d ไม่สำเร็จ: %v", serviceID, err)
	}
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

// translate แปลงคำตอบของคลัสเตอร์เป็นสถานะที่ระบบเราเก็บ
//
// แยกออกมาเป็นฟังก์ชันล้วนๆ เพื่อให้เทสต์ได้โดยไม่ต้องมีทั้ง DB และคลัสเตอร์
func translate(s WorkloadStatus) (status, reason, message string) {
	switch s.Phase {
	case PhaseRunning:
		// หายดีแล้วต้องล้าง reason/message เก่าทิ้งด้วย ไม่งั้นหน้าเว็บจะยังโชว์
		// ข้อความ crash loop ของเมื่อกี้ค้างอยู่บน service ที่กลับมารันปกติแล้ว
		return entity.ServiceRunning, "", ""

	case PhaseCrashLoop:
		return entity.ServiceCrashLoop, orDefault(s.Reason, "CrashLoopBackOff"), s.Message

	case PhasePending:
		return entity.ServicePending, orDefault(s.Reason, "Pending"), s.Message

	case PhaseGone:
		return entity.ServiceFailed, orDefault(s.Reason, "NotFound"),
			orDefault(s.Message, "ไม่พบ workload นี้บนคลัสเตอร์แล้ว — อาจถูกลบจากนอกระบบ")

	default: // PhaseFailed และค่าที่ไม่รู้จัก
		return entity.ServiceFailed, orDefault(s.Reason, "Failed"), s.Message
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
