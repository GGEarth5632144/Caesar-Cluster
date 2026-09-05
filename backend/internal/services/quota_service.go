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
	ErrServiceTooLarge = errors.New("สเปกที่ขอเกินเพดานของ service 1 ตัว")
)

// NamespaceUsage = ยอดใช้งานจริงของ namespace ณ ตอนนี้ (คำนวณสดจากตาราง services ทุกครั้ง ไม่เก็บซ้ำ)
//
// UsedCPUMilli/UsedRAMMB เป็นยอดรวมทุก Pod แล้ว คือ SUM(cpu_milli × replicas)
// ส่วน ServiceCount นับเป็นจำนวน service ไม่ใช่จำนวน Pod เพราะเป็นหน่วยที่ผู้ใช้เห็นบนหน้าเว็บ
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
		Select(`COALESCE(SUM(cpu_milli * replicas), 0) AS used_cpu_milli,
		        COALESCE(SUM(ram_mb * replicas), 0)    AS used_ram_mb,
		        COUNT(*)                               AS service_count`).
		Where("namespace_id = ?", namespaceID).
		Scan(&u).Error
	return u, err
}

// usageExcluding เหมือน Usage แต่ไม่นับ service ตัวที่ระบุ — ใช้ตอน scale ถามว่า "ถ้าไม่มีตัวนี้ เหลือเท่าไหร่"
// ถ้าใช้ Usage ตรงๆ ยอดเดิมของมันจะถูกนับซ้ำ ทำให้ scale ลงโดนบล็อกทั้งที่ควรผ่าน
func (q *QuotaService) usageExcluding(ctx context.Context, tx *gorm.DB, namespaceID, excludeServiceID int) (NamespaceUsage, error) {
	db := tx
	if db == nil {
		db = q.db.WithContext(ctx)
	}

	var u NamespaceUsage
	err := db.Table("services").
		Select(`COALESCE(SUM(cpu_milli * replicas), 0) AS used_cpu_milli,
		        COALESCE(SUM(ram_mb * replicas), 0)    AS used_ram_mb,
		        COUNT(*)                               AS service_count`).
		Where("namespace_id = ? AND id <> ?", namespaceID, excludeServiceID).
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
// replicas คูณกับสเปกต่อ Pod ก่อนเทียบโควตา namespace แต่ "ไม่" คูณตอนเทียบเพดานของ service เดี่ยว
// เพราะเพดานนั้นคุมขนาด Pod 1 ตัว ส่วนการทำซ้ำ Pod เป็นเรื่องของโควตารวม
//
// นี่เป็น pattern เดียวกับ AllocationService เดิมเป๊ะๆ แค่เปลี่ยนของที่ล็อกจาก node เป็น namespace
func (q *QuotaService) ReserveAndInsert(
	ctx context.Context,
	namespaceID, cpuMilli, ramMB, replicas int,
	insert func(tx *gorm.DB) error,
) error {

	// เพดานของ service ตัวเดียว — เช็คก่อนเลย ไม่ต้องเปิด transaction ให้เปลือง
	if cpuMilli > entity.MaxCPUMilliPerService || ramMB > entity.MaxRAMMBPerService {
		return fmt.Errorf("%w: สูงสุด %dm CPU / %d MB ต่อ 1 service",
			ErrServiceTooLarge, entity.MaxCPUMilliPerService, entity.MaxRAMMBPerService)
	}
	if replicas < entity.MinReplicas || replicas > entity.MaxReplicas {
		return fmt.Errorf("%w: replica ต้องอยู่ระหว่าง %d-%d",
			ErrServiceTooLarge, entity.MinReplicas, entity.MaxReplicas)
	}

	totalCPU, totalRAM := cpuMilli*replicas, ramMB*replicas

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

		if used.UsedCPUMilli+totalCPU > ns.CPULimitMilli {
			return fmt.Errorf("%w: CPU เหลือ %dm แต่ขอ %dm (%dm × %d replica)",
				ErrQuotaExceeded, ns.CPULimitMilli-used.UsedCPUMilli, totalCPU, cpuMilli, replicas)
		}
		if used.UsedRAMMB+totalRAM > ns.RAMLimitMB {
			return fmt.Errorf("%w: RAM เหลือ %d MB แต่ขอ %d MB (%d MB × %d replica)",
				ErrQuotaExceeded, ns.RAMLimitMB-used.UsedRAMMB, totalRAM, ramMB, replicas)
		}

		// โควตาพอ → ให้ผู้เรียก INSERT service ภายใน tx เดียวกับที่ล็อก namespace ไว้
		return insert(tx)
	})
}

// ReserveScale = คู่แฝดของ ReserveAndInsert แต่สำหรับของที่ deploy ไปแล้ว: ล็อก namespace →
// นับยอดของทุก service ยกเว้นตัวนี้ → เทียบกับสเปกใหม่ → ผ่านแล้วให้ผู้เรียก UPDATE ใน tx เดียวกัน
//
// ต้องล็อกด้วยเหตุผลเดียวกับตอน insert: ถ้าไม่ล็อก scale กับ deploy ที่มาพร้อมกัน
// จะอ่านยอดเดิมแล้วต่างคนต่างคิดว่าโควตาพอ
func (q *QuotaService) ReserveScale(
	ctx context.Context,
	namespaceID, serviceID, cpuMilli, ramMB, replicas int,
	update func(tx *gorm.DB) error,
) error {

	if replicas < entity.MinReplicas || replicas > entity.MaxReplicas {
		return fmt.Errorf("%w: replica ต้องอยู่ระหว่าง %d-%d",
			ErrServiceTooLarge, entity.MinReplicas, entity.MaxReplicas)
	}

	totalCPU, totalRAM := cpuMilli*replicas, ramMB*replicas

	return q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ns entity.Namespace
		if err := tx.Raw(`SELECT * FROM namespaces WHERE id = ? FOR UPDATE`, namespaceID).
			Scan(&ns).Error; err != nil {
			return err
		}
		if ns.ID == 0 {
			return ErrNoNamespace
		}

		// ยอดของ service ตัวอื่น — ยอดเดิมของตัวที่กำลังจะ scale ต้องไม่ถูกนับซ้ำ
		others, err := q.usageExcluding(ctx, tx, namespaceID, serviceID)
		if err != nil {
			return err
		}

		if others.UsedCPUMilli+totalCPU > ns.CPULimitMilli {
			return fmt.Errorf("%w: CPU เหลือ %dm แต่ %d replica ต้องใช้ %dm",
				ErrQuotaExceeded, ns.CPULimitMilli-others.UsedCPUMilli, replicas, totalCPU)
		}
		if others.UsedRAMMB+totalRAM > ns.RAMLimitMB {
			return fmt.Errorf("%w: RAM เหลือ %d MB แต่ %d replica ต้องใช้ %d MB",
				ErrQuotaExceeded, ns.RAMLimitMB-others.UsedRAMMB, replicas, totalRAM)
		}

		return update(tx)
	})
}
