package entity

import "time"

// ประเภทเหตุการณ์ที่ audit log บันทึก
const (
	AuditEventCreate  = "CREATE"
	AuditEventApprove = "APPROVE"
	AuditEventUpdate  = "UPDATE"
)

// AuditLog = ตาราง audit_logs — บันทึกทุกการกระทำสำคัญในระบบไว้ตรวจสอบย้อนหลัง (ใครทำอะไร เมื่อไหร่ จากที่ไหน)
//
// ข้อมูลไหลเข้า: ทุก controller ที่ทำ action สำคัญ (สร้าง namespace, อนุมัติ/ปฏิเสธ request, แก้โควตา ฯลฯ)
// เขียน log เข้ามาหลัง action สำเร็จ — เก็บ snapshot ของ actor (role/name) ไว้ตรงนี้เลย ไม่ FK ไปตาราง users
// เพราะถ้า user ถูกลบหรือเปลี่ยนชื่อทีหลัง ประวัติ audit ต้องยังอ่านได้เหมือนเดิม ไม่เปลี่ยนย้อนหลัง
// ข้อมูลไหลออก: AdminController โชว์หน้า audit trail ให้ admin ตรวจสอบ
type AuditLog struct {
	ID          int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	EventType   string    `gorm:"column:event_type;type:varchar(20);not null;check:event_type IN ('CREATE','APPROVE','UPDATE')" json:"event_type"`
	ActorRole   string    `gorm:"column:actor_role;type:varchar(20);not null" json:"actor_role"`
	ActorName   string    `gorm:"column:actor_name;type:varchar(100);not null" json:"actor_name"`
	ActionTitle string    `gorm:"column:action_title;type:varchar(100);not null" json:"action_title"`
	Detail      string    `gorm:"column:detail;type:text" json:"detail"`
	SourceIP    string    `gorm:"column:source_ip;type:varchar(45);not null" json:"source_ip"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "audit_logs"
func (AuditLog) TableName() string { return "audit_logs" }
