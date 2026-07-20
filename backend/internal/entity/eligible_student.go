package entity

import "time"

// MajorCPE = ชื่อสาขาที่ระบบนี้เปิดให้สมัครได้ ("Cloud for CPE Students")
// data flow: AuthController.Register เทียบ eligible.Major กับค่านี้ เป็นด่านที่ 2 ต่อจากเช็คว่ามี student_id ในระบบไหม
const MajorCPE = "Computer Engineering"

// EligibleStudent = ตาราง eligible_students (คือตาราง "match" ใน ERD)
// เก็บรายชื่อ นศ. ที่รู้จัก (ทุกสาขา ไม่ใช่แค่ CPE) พร้อม major ของแต่ละคน
//
// ข้อมูลไหลเข้า: admin import รายชื่อเข้ามาผ่าน POST /api/admin/eligible-students
// ข้อมูลไหลออก: AuthController.Register เช็ค 2 ชั้นก่อนให้สมัคร:
//  1. มี student_id นี้อยู่ในตารางไหม (ถ้าไม่มี → 403 STUDENT_NOT_FOUND)
//  2. ถ้ามี, Major ตรงกับ MajorCPE ไหม (ถ้าไม่ตรง → 403 NOT_CPE — เจอตัว แต่ไม่ใช่สาขา CPE สมัครไม่ได้)
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
