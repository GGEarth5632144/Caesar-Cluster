package dto

import "backend/internal/entity"

// RegisterRequest = body ของ POST /api/register
// data flow: JSON จาก client → ShouldBindJSON ใน AuthController.Register
// → เช็ค student_id กับตาราง eligible_students ก่อน → ถ้าผ่านค่อยสร้าง entity.User
type RegisterRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	RealName  string `json:"real_name" binding:"required"`
	Gmail     string `json:"gmail" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8"`
}

// LoginRequest = body ของ POST /api/login
// data flow: JSON จาก client → ShouldBindJSON ใน AuthController.Login → ใช้ค้นหา user + เทียบรหัสผ่าน
// Remember = ติ๊ก "Remember For 30 Days" มาไหม — คุม exp ของ JWT ที่ออกให้ (ดู AuthController.Login)
type LoginRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	Password  string `json:"password" binding:"required"`
	Remember  bool   `json:"remember"`
}

// VerifyEmailRequest = body ของ POST /api/verify-email (token มาจาก query ของลิงก์ในอีเมล)
//
// ให้หน้าเว็บยิง POST แทนที่จะให้ลิงก์ชี้มาที่ backend ตรงๆ ด้วย GET เพราะตัวสแกนลิงก์ของ
// Gmail/Outlook กดลิงก์ให้ก่อนผู้ใช้เสมอ ถ้าลิงก์นั้นเผาโทเคนทิ้ง ผู้ใช้จะเจอ "ลิงก์ถูกใช้แล้ว" ทุกครั้ง
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ResendVerificationRequest = body ของ POST /api/resend-verification
// หา user ที่ยังไม่ยืนยันจาก gmail → ออกลิงก์ใบใหม่ (ตอบข้อความ generic เสมอ เหมือน /forgot-password)
type ResendVerificationRequest struct {
	Gmail string `json:"gmail" binding:"required,email"`
}

type UpdateUserRequest struct {
	StudentID *string `json:"student_id"`
	RealName  *string `json:"real_name"`
	Gmail     *string `json:"gmail" binding:"omitempty,email"`
	RoleID    *int    `json:"role_id"`
}

// ForgotPasswordRequest = body ของ POST /api/forgot-password
// data flow: JSON จาก client → AuthController.ForgotPassword → หา user จาก gmail → ส่งลิงก์รีเซ็ตไปทางอีเมล
type ForgotPasswordRequest struct {
	Gmail string `json:"gmail" binding:"required,email"`
}

// ResetPasswordRequest = body ของ POST /api/reset-password
// data flow: JSON จาก client (token มาจาก query ในลิงก์อีเมล) → AuthController.ResetPassword
// → ตรวจ token → ตั้งรหัสผ่านใหม่ (min=8 ให้ตรงกับ RegisterRequest.Password)
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// UpdateProfileRequest = body ของ PATCH /api/me
type UpdateProfileRequest struct {
	RealName string `json:"real_name" binding:"required,max=100"`
}

// ChangePasswordRequest = body ของ POST /api/me/password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

// UserWithNamespace = entity.User + โควตาของ namespace ที่ผู้ใช้สังกัด (โควตาผูกกับ namespace ไม่ใช่ user แล้ว)
// ใช้ตอบ ListUsers/UpdateUser ของ AdminController
//
// CPULimitMilli/RAMLimitMB ดึงมาจาก namespace ของผู้ใช้ (ถ้ายังไม่มี space จะเป็น 0)
//
// NamespaceName คือชื่อ space ที่ผู้ใช้สังกัด — หน้า User Management โชว์ชื่อนี้แทนโควตา
// (โควตาย้ายไปจัดการที่หน้า Namespace Management แล้ว) ผู้ใช้ที่ยังไม่มี space จะเป็นค่าว่าง
type UserWithNamespace struct {
	entity.User
	NamespaceName string `json:"namespace_name"`
	CPULimitMilli int    `json:"cpu_limit_milli"`
	RAMLimitMB    int    `json:"ram_limit_mb"`
}
