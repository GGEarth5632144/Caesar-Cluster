package entity

import "time"

// EligibleStudent = ตาราง eligible_students (คือตาราง "match" ใน ERD)
// มีไว้ตรวจว่า student_id ที่มาสมัครเป็น นศ. ในสาขาที่เราอนุญาตจริงๆ — ระบบนี้เปิดให้เฉพาะกลุ่มที่จำกัดไว้
//
// ข้อมูลไหลเข้า: admin import รายชื่อเข้ามาผ่าน POST /api/admin/eligible-students
// ข้อมูลไหลออก: AuthController.Register เช็คก่อนสมัครว่ามี student_id นี้อยู่ในตารางไหม
// ถ้าไม่มี → 403 สมัครไม่ได้
//
// นอกจากเช็คในโค้ดแล้ว ยังมี FK users.student_id → eligible_students.student_id กันอีกชั้นที่ระดับ DB
// (ต่อให้มีใครลืมเช็คในโค้ด ก็ยัง insert user ที่ไม่อยู่ในรายชื่อไม่ได้อยู่ดี)
type EligibleStudent struct {
	StudentID string    `gorm:"column:student_id;type:varchar(20);primaryKey" json:"student_id"`
	Major     string    `gorm:"column:major;type:varchar(100);not null" json:"major"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "eligible_students"
func (EligibleStudent) TableName() string { return "eligible_students" }
