package dto

import "backend/internal/entity"

// RegisterRequest = body ของ POST /api/register
// data flow: JSON จาก client → ShouldBindJSON ใน AuthController.Register
// → เช็ค student_id กับตาราง eligible_students ก่อน → ถ้าผ่านค่อยสร้าง entity.User
type RegisterRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	RealName  string `json:"real_name" binding:"required"`
	Gmail	  string `json:"gmail" binding:"required,email"`
	NickName  string `json:"nick_name"`
	Password  string `json:"password" binding:"required,min=8"`
}

// LoginRequest = body ของ POST /api/login
// data flow: JSON จาก client → ShouldBindJSON ใน AuthController.Login → ใช้ค้นหา user + เทียบรหัสผ่าน
type LoginRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

type UpdateUserRequest struct {
	StudentID *string `json:"student_id"`
	RealName  *string `json:"real_name"`
	Gmail     *string `json:"gmail" binding:"omitempty,email"`
	NickName  *string `json:"nick_name"`
	Year      *int    `json:"year"`
	RoleID    *int    `json:"role_id"`
}

// UserWithYearLevel = entity.User + ชั้นปีที่คำนวณสดจาก student_id (entity.YearLevel)
// + โควตาของ namespace ที่ผู้ใช้สังกัด (โควตาผูกกับ namespace ไม่ใช่ user แล้ว)
//
// ให้หน้า admin โชว์เป็น "Year 4" ได้ตรงๆ แทนที่จะโชว์ User.EntryYear (ปีที่เข้าศึกษา พ.ศ. เช่น 2566)
// ซึ่งเป็นคนละความหมายกัน — ใช้ตอน ListUsers เท่านั้น (ดู AdminController.ListUsers)
//
// CPULimitMilli/RAMLimitMB ดึงมาจาก namespace ของผู้ใช้ (ถ้ายังไม่มี space จะเป็น 0)
type UserWithYearLevel struct {
	entity.User
	YearLevel     int `json:"year_level"`
	CPULimitMilli int `json:"cpu_limit_milli"`
	RAMLimitMB    int `json:"ram_limit_mb"`
}