package dto

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
	CPUlimit  *int    `json:"cpu_limit"`
	Ramlimit  *int    `json:"ram_limit"`
}