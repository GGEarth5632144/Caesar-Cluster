package entity

import "time"

// สถานะของ request — user ยื่นแล้วต้องรอ admin ตัดสิน
const (
	RequestPending  = "pending"
	RequestApproved = "approved"
	RequestDenied   = "denied"
)

// Request = ตาราง requests (คือ request ใน ERD) — คำขอสร้าง namespace/ขอโควตาที่ user ยื่นเข้ามา
// ต้องรอ admin อนุมัติก่อนถึงจะสร้าง Namespace จริง (ดู namespace.go)
//
// namespace_name/cpu_limit_milli/ram_limit_mb เป็นค่าที่ user "ขอ" ไว้เฉยๆ ยังไม่ใช่ namespace จริง
// (กรอกเองหรือก๊อปมาจาก RequestTemplate ก็ได้ — เป็น snapshot เหมือนกัน ดูเหตุผลใน plan.go)
// ถ้า admin approve ค่อยเอาค่าพวกนี้ไปสร้างแถวใน namespaces จริงอีกที โดย Namespace.ContributorID = Request.UserID
// (ERD มีฟิลด์ name_space.ContributorID แยกไว้ แต่ในระบบนี้ "1 คน = 1 space" คนที่ยื่นคือเจ้าของเสมอ
// จึงตัดออกเพราะจะซ้ำกับ user_id เฉยๆ)
//
// ข้อมูลไหลเข้า: user ยื่นผ่าน RequestController.Create
// ข้อมูลไหลออก: AdminController อ่านคำขอที่ status = pending มาอนุมัติ/ปฏิเสธ
// ถ้า approve → ไปสร้าง Namespace จริงแล้วอัปเดต status กลับมาเป็น approved
type Request struct {
	ID                int    `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Description       string `gorm:"column:description;type:text" json:"description"`
	UserID            int    `gorm:"column:user_id;type:integer;not null;index:idx_requests_user" json:"user_id"`
	Status            string `gorm:"column:status;type:varchar(10);not null;default:pending;check:status IN ('pending','approved','denied')" json:"status"`
	NamespaceName     string `gorm:"column:namespace_name;type:varchar(50);not null" json:"namespace_name"`
	RequestTemplateID *int   `gorm:"column:request_template_id;type:integer" json:"request_template_id"`
	CPULimitMilli     int    `gorm:"column:cpu_limit_milli;type:integer;not null;check:cpu_limit_milli > 0" json:"cpu_limit_milli"`
	RAMLimitMB        int    `gorm:"column:ram_limit_mb;type:integer;not null;check:ram_limit_mb > 0" json:"ram_limit_mb"`
	// StorageGB เป็น snapshot ที่ก๊อปมาจาก RequestTemplate ตอนยื่นคำขอ (เหตุผลเดียวกับ Service.CPUMilli/RAMMB
	// ดู request_template.go) เป็น 0 ได้ถ้าคำขอนี้ไม่ได้อ้างอิง template ใดเลย
	StorageGB int       `gorm:"column:storage_gb;type:integer;not null;default:0" json:"storage_gb"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "requests"
func (Request) TableName() string { return "requests" }
