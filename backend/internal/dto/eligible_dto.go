package dto

import "backend/internal/entity"

// EligibleStudentWithYearLevel = entity.EligibleStudent + ชั้นปีที่คำนวณสดจาก student_id (entity.YearLevel)
// ใช้ตอบ GET /api/admin/eligible-students — หน้า "รายชื่อผู้มีสิทธิ์" โชว์ชั้นปีแทนชื่อ-สกุล
// รหัสที่แกะปีไม่ได้จะได้ 0 (หน้าเว็บโชว์เป็นขีด) เหมือน UserWithYearLevel
type EligibleStudentWithYearLevel struct {
	entity.EligibleStudent
	YearLevel int `json:"year_level"`
}

// AddEligibleStudentRequest = body ของ POST /api/admin/eligible-students
// admin ใช้ยืนยัน (confirm) การ import รายชื่อ นศ. ที่มีสิทธิ์สมัคร (ตาราง "match" ใน ERD)
// รับเป็น array เพื่อ import ทีละหลายคนได้ในครั้งเดียว — ปกติคือ "valid" list ที่ได้จาก
// POST /api/admin/eligible-students/preview ส่งกลับมายืนยันอีกที (ดู PreviewEligibleStudentsResponse)
// data flow: JSON จาก client → AdminController.AddEligibleStudents → UPSERT eligible_students
// (ซ้ำ student_id เดิม → อัปเดต major/real_name/enrollment_status ให้ตรงไฟล์ล่าสุด)
type AddEligibleStudentRequest struct {
	Students []EligibleStudentItem `json:"students" binding:"required,min=1,dive"`
}

// AddSingleEligibleStudentRequest = body ของ POST /api/admin/eligible-students/single — แอดมินเพิ่มทีละคนเอง
// ต่างจาก import ตรงที่กำหนด role ได้ และไม่มีชื่อ (ไม่เขียนทับชื่อเดิมที่ import มาจากไฟล์)
// data flow: JSON จาก client → AdminController.AddEligibleStudent → UPSERT eligible_students (+ users.role_id)
type AddSingleEligibleStudentRequest struct {
	StudentID        string `json:"student_id" binding:"required,min=3,max=20"`
	Major            string `json:"major" binding:"required,min=2,max=100"`
	EnrollmentStatus int    `json:"enrollment_status" binding:"required"`
	Role             string `json:"role" binding:"required,oneof=user admin"`
}

// EligibleStudentItem = 1 รายชื่อในลิสต์ที่ import เข้ามา (แถวหนึ่งจากไฟล์ Excel ของทะเบียน)
type EligibleStudentItem struct {
	StudentID        string `json:"student_id" binding:"required,min=3,max=20"`
	RealName         string `json:"real_name"`
	Major            string `json:"major" binding:"required,min=2,max=100"`
	EnrollmentStatus int    `json:"enrollment_status" binding:"required"`
}

// InvalidEligibleRow = แถวในไฟล์ Excel ที่ parse ไม่ผ่าน (ไม่มี student_id, major ว่าง, สถานภาพอ่านไม่ออก ฯลฯ)
// Row คือเลขแถวในไฟล์ Excel (นับรวม header) ไว้ให้ admin กลับไปเช็คไฟล์ต้นฉบับได้ตรงจุด
type InvalidEligibleRow struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

// EligibleImportSummary = สรุปผลกระทบก่อน apply จริง ให้ admin เห็นภาพก่อนกด confirm
type EligibleImportSummary struct {
	New       int `json:"new"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// PreviewEligibleStudentsResponse = ผลลัพธ์ของ POST /api/admin/eligible-students/preview
// data flow: อัปโหลดไฟล์ .xlsx → AdminController.PreviewEligibleStudents parse + validate +
// เทียบกับข้อมูลเดิมใน eligible_students → ตอบกลับมาให้ frontend แสดง preview
// Valid ส่งกลับให้ frontend เก็บไว้ แล้วส่งต่อเป็น AddEligibleStudentRequest.Students
// ตอน admin กด confirm (endpoint นี้ไม่เก็บ state ฝั่ง server เลย)
type PreviewEligibleStudentsResponse struct {
	Valid   []EligibleStudentItem `json:"valid"`
	Invalid []InvalidEligibleRow  `json:"invalid"`
	Summary EligibleImportSummary `json:"summary"`
}
