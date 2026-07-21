package entity

import "time"

// MajorCPE = ชื่อสาขาที่ระบบนี้เปิดให้สมัครได้ ("CPE" ตามค่าจริงในไฟล์ export ของทะเบียน)
// data flow: AuthController.Register เทียบ eligible.Major กับค่านี้ เป็นด่านที่ 2 ต่อจากเช็คว่ามี student_id ในระบบไหม
const MajorCPE = "CPE"

// ActiveEnrollmentStatuses = ค่า สถานภาพ ที่ยังอนุญาตให้สมัคร/ใช้งานระบบได้
// อ้างอิงรหัสจากไฟล์ export ของทะเบียน: 10=กำลังศึกษา, 11=รักษาสภาพการเป็นนักศึกษา
// (12=ลาพัก, 13=ให้พัก, 40=สำเร็จการศึกษา, 60-89=สิ้นสุดสถานภาพ ไม่อนุญาต)
var ActiveEnrollmentStatuses = map[int]bool{
	10: true,
	11: true,
}

// EligibleStudent = ตาราง eligible_students (คือตาราง "match" ใน ERD)
// เก็บรายชื่อ นศ. ที่รู้จัก (ทุกสาขา ไม่ใช่แค่ CPE) พร้อม major/สถานภาพของแต่ละคน
//
// ข้อมูลไหลเข้า: admin import ไฟล์ Excel จากทะเบียนผ่าน POST /api/admin/eligible-students/preview
// (parse + validate) แล้วยืนยันผ่าน POST /api/admin/eligible-students (upsert จริง)
// ข้อมูลไหลออก: AuthController.Register เช็ค 3 ชั้นก่อนให้สมัคร:
//  1. มี student_id นี้อยู่ในตารางไหม (ถ้าไม่มี → 403 STUDENT_NOT_FOUND)
//  2. ถ้ามี, Major ตรงกับ MajorCPE ไหม (ถ้าไม่ตรง → 403 NOT_CPE — เจอตัว แต่ไม่ใช่สาขา CPE สมัครไม่ได้)
//  3. EnrollmentStatus อยู่ใน ActiveEnrollmentStatuses ไหม (ถ้าไม่ → 403 NOT_ACTIVE_STUDENT)
//
// นอกจากเช็คในโค้ดแล้ว ยังมี FK users.student_id → eligible_students.student_id กันอีกชั้นที่ระดับ DB
// (ต่อให้มีใครลืมเช็คในโค้ด ก็ยัง insert user ที่ไม่อยู่ในรายชื่อไม่ได้อยู่ดี)
//
// สำคัญ: แถวในตารางนี้ "ไม่ถูกลบ" เมื่อ นศ. จบ/พ้นสภาพ เพราะ users.student_id มี FK อ้างมาที่นี่ —
// ลบแถวของคนที่สมัครไปแล้วจะทำให้ FK พัง (หรือถ้า cascade จะลบ user ทิ้งไปด้วย ซึ่งไม่ใช่สิ่งที่ต้องการ)
// การ import ไฟล์ใหม่ทุกเทอมจึงเป็นแค่ upsert — EnrollmentStatus จะถูกอัปเดตให้ตรงกับไฟล์ล่าสุดแทน
type EligibleStudent struct {
	StudentID        string    `gorm:"column:student_id;type:varchar(20);primaryKey" json:"student_id"`
	RealName         string    `gorm:"column:real_name;type:varchar(150)" json:"real_name"`
	Major            string    `gorm:"column:major;type:varchar(100);not null" json:"major"`
	EnrollmentStatus int       `gorm:"column:enrollment_status;type:integer;not null;default:10" json:"enrollment_status"`
	ImportedAt       time.Time `gorm:"column:imported_at;type:timestamp;not null;default:now()" json:"imported_at"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "eligible_students"
func (EligibleStudent) TableName() string { return "eligible_students" }
