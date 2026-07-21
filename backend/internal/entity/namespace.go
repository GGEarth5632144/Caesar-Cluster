package entity

import "time"

// เพดานทรัพยากร — ตัวเลขทั้งหมดมาจาก spec ที่คุยกันไว้
// CPU เก็บเป็น "millicore" (1 core = 1000m) เพื่อให้เลือกเป็น % ได้ เช่น 300% = 3000m, 50% = 500m
const (
	// โควตาตั้งต้นของทุก namespace ที่เพิ่งสร้าง — เป็นยอด "รวมทั้ง namespace" ไม่ใช่ต่อ service
	DefaultCPULimitMilli = 3000 // 3 core (300%)
	DefaultRAMLimitMB    = 2048 // 2 GB

	// เพดานที่ admin ปรับโควตาให้ได้สูงสุด (ทุก namespace เท่ากันหมด หลังเลิกแยกชนิด solo/group)
	MaxCPULimitMilli = 8000 // 8 core
	MaxRAMLimitMB    = 8192 // 8 GB

	// เพดานของ service เดี่ยวๆ 1 ตัว (ต่อให้ namespace มีโควตาเหลือ ก็ขอเกินนี้ไม่ได้)
	MaxCPUMilliPerService = 3000 // 300%
	MaxRAMMBPerService    = 2048 // 2 GB
)

// Namespace = ตาราง namespaces (คือ name_space / group ใน ERD) — "หน่วยที่ถือโควตา" ของระบบนี้
//
// สำคัญ: โควตาผูกกับ namespace ไม่ใช่กับ node — เพราะ Kubernetes เป็นคนเลือก node ให้เอง
// หน้าที่ของ backend เราคือคุมว่า namespace นี้ใช้ทรัพยากรรวมกันได้ไม่เกินเท่าไหร่ (ResourceQuota)
//
// ข้อมูลไหลเข้า: NamespaceManager.Create (user สร้าง space ของตัวเอง/กลุ่ม)
// หรือ AdminController ปรับโควตาให้ทีหลัง
// ข้อมูลไหลออก: QuotaService อ่าน limit ทั้ง 3 ตัวไปเทียบก่อนอนุญาตให้ deploy service ใหม่,
// Provisioner.EnsureNamespace เอาไปสร้าง namespace + ResourceQuota จริงบน k8s
type Namespace struct {
	ID            int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Name          string    `gorm:"column:name;type:varchar(50);unique;not null" json:"name"`
	ContributorID int       `gorm:"column:contributor_id;type:integer;not null;index:idx_namespaces_contributor" json:"contributor_id"`
	CPULimitMilli int       `gorm:"column:cpu_limit_milli;type:integer;not null;check:cpu_limit_milli > 0" json:"cpu_limit_milli"`
	RAMLimitMB    int       `gorm:"column:ram_limit_mb;type:integer;not null;check:ram_limit_mb > 0" json:"ram_limit_mb"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "namespaces"
func (Namespace) TableName() string { return "namespaces" }
