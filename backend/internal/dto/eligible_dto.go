package dto

// AddEligibleStudentRequest = body ของ POST /api/admin/eligible-students
// admin ใช้ import รายชื่อ นศ. ที่มีสิทธิ์สมัคร (ตาราง "match" ใน ERD)
// รับเป็น array เพื่อ import ทีละหลายคนได้ในครั้งเดียว
// data flow: JSON จาก client → AdminController.AddEligibleStudents → INSERT eligible_students (ข้ามตัวที่ซ้ำ)
type AddEligibleStudentRequest struct {
	Students []EligibleStudentItem `json:"students" binding:"required,min=1,dive"`
}

// EligibleStudentItem = 1 รายชื่อในลิสต์ที่ import เข้ามา
type EligibleStudentItem struct {
	StudentID string `json:"student_id" binding:"required,min=3,max=20"`
	Major     string `json:"major" binding:"required,min=2,max=100"`
}
