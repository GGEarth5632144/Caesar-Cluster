package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// error ที่ controller เอาไปแปลงเป็น HTTP status ได้ (errors.Is)
var (
	ErrNoNamespace     = errors.New("ยังไม่มี namespace — ต้องสร้างหรือเข้ากลุ่มก่อน")
	ErrQuotaExceeded   = errors.New("ทรัพยากรที่ขอเกินโควตาที่เหลือของ namespace")
	ErrServiceLimit    = errors.New("จำนวน service ใน namespace เต็มแล้ว")
	ErrServiceTooLarge = errors.New("สเปกที่ขอเกินเพดานของ service 1 ตัว")
)

// NamespaceUsage = ยอดใช้งานจริงของ namespace ณ ตอนนี้ (คำนวณสดจากตาราง services ทุกครั้ง ไม่เก็บซ้ำ)
type NamespaceUsage struct {
	UsedCPUMilli int `json:"used_cpu_milli"`
	UsedRAMMB    int `json:"used_ram_mb"`
	ServiceCount int `json:"service_count"`
}

// QuotaService รับผิดชอบเรื่องเดียว: บังคับโควตาของ namespace
//
// นี่คือตัวที่มาแทน AllocationService เดิม (ที่เอาไว้ไล่หา node ว่าง)
// บน Kubernetes เราไม่ต้องเลือก node เอง — scheduler ของ k8s ทำให้ — หน้าที่ที่เหลือของ backend
// คือคุมว่า "namespace นี้ใช้รวมกันได้ไม่เกินเท่าไหร่" ซึ่งก็คือไฟล์นี้
type QuotaService struct{ db *gorm.DB }

// NewQuotaService ประกอบ service — ถูกเรียกจาก main ตอน start
func NewQuotaService(db *gorm.DB) *QuotaService {
	return &QuotaService{db: db}
}

// Usage คืนยอดใช้งานปัจจุบันของ namespace (SUM cpu/ram + COUNT service)
// data flow: รับ namespaceID จาก NamespaceManager/ServiceManager → SUM จากตาราง services → คืน NamespaceUsage
// ใช้ tx ที่ส่งเข้ามาได้ (ตอนอยู่ใน transaction) หรือส่ง nil เพื่อใช้ connection ปกติ
func (q *QuotaService) Usage(ctx context.Context, tx *gorm.DB, namespaceID int) (NamespaceUsage, error) {
	db := tx
	if db == nil {
		db = q.db.WithContext(ctx)
	}

	var u NamespaceUsage
	err := db.Table("services").
		Select(`COALESCE(SUM(cpu_milli), 0) AS used_cpu_milli,
		        COALESCE(SUM(ram_mb), 0)    AS used_ram_mb,
		        COUNT(*)                    AS service_count`).
		Where("namespace_id = ?", namespaceID).
		Scan(&u).Error
	return u, err
}

// ReserveAndInsert คือหัวใจของการกันใช้เกินโควตา: เช็คโควตา + INSERT service ภายใน transaction เดียวกัน
//
// data flow:
//   - รับ namespaceID + สเปกที่ขอ (cpuMilli, ramMB) + callback insert จาก ServiceManager.Create
//   - เปิด transaction แล้ว SELECT namespace ... FOR UPDATE เพื่อ "ล็อกแถว namespace" ไว้ก่อน
//   - นับยอดใช้จริงของ namespace (SUM services) แล้วเทียบกับ limit ทั้ง 3 ตัว
//   - ผ่านทุกข้อ → เรียก insert(tx) ให้ ServiceManager INSERT service ภายใน tx เดียวกัน
//
// ทำไมต้องล็อกแถว namespace: ถ้า 2 request ขอ deploy พร้อมกัน ทั้งคู่จะอ่านยอดใช้เดิม (เช่น 0)
// แล้วต่างคนต่างคิดว่าโควตาพอ → ใช้เกิน (overcommit) การ FOR UPDATE ทำให้คนที่สองต้องรอ
// แล้วเห็นยอดที่คนแรก INSERT ไปแล้ว จึงคำนวณถูก
//
// นี่เป็น pattern เดียวกับ AllocationService เดิมเป๊ะๆ แค่เปลี่ยนของที่ล็อกจาก node เป็น namespace
func (q *QuotaService) ReserveAndInsert(
	ctx context.Context,
	namespaceID, cpuMilli, ramMB int,
	insert func(tx *gorm.DB) error,
) error {

	// เพดานของ service ตัวเดียว — เช็คก่อนเลย ไม่ต้องเปิด transaction ให้เปลือง
	if cpuMilli > entity.MaxCPUMilliPerService || ramMB > entity.MaxRAMMBPerService {
		return fmt.Errorf("%w: สูงสุด %dm CPU / %d MB ต่อ 1 service",
			ErrServiceTooLarge, entity.MaxCPUMilliPerService, entity.MaxRAMMBPerService)
	}

	return q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ล็อกแถว namespace ไว้จนจบ transaction — กัน request อื่นเช็คโควตาพร้อมกันแล้วใช้เกิน
		var ns entity.Namespace
		if err := tx.Raw(`SELECT * FROM namespaces WHERE id = ? FOR UPDATE`, namespaceID).
			Scan(&ns).Error; err != nil {
			return err
		}
		if ns.ID == 0 {
			return ErrNoNamespace
		}

		used, err := q.Usage(ctx, tx, namespaceID)
		if err != nil {
			return err
		}

		if used.ServiceCount >= ns.MaxServices {
			return fmt.Errorf("%w: deploy ได้สูงสุด %d services", ErrServiceLimit, ns.MaxServices)
		}
		if used.UsedCPUMilli+cpuMilli > ns.CPULimitMilli {
			return fmt.Errorf("%w: CPU เหลือ %dm แต่ขอ %dm",
				ErrQuotaExceeded, ns.CPULimitMilli-used.UsedCPUMilli, cpuMilli)
		}
		if used.UsedRAMMB+ramMB > ns.RAMLimitMB {
			return fmt.Errorf("%w: RAM เหลือ %d MB แต่ขอ %d MB",
				ErrQuotaExceeded, ns.RAMLimitMB-used.UsedRAMMB, ramMB)
		}

		// โควตาพอ → ให้ผู้เรียก INSERT service ภายใน tx เดียวกับที่ล็อก namespace ไว้
		return insert(tx)
	})
}
