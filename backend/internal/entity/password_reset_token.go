package entity

import "time"

// PasswordResetToken = ตาราง password_reset_tokens — โทเคนสำหรับรีเซ็ตรหัสผ่านผ่านอีเมล
//
// เก็บเฉพาะ "hash" ของ token (sha256 hex) ไม่เคยเก็บ token ตัวจริงลง DB — หลักการเดียวกับ User.Password
// ที่เก็บ bcrypt hash: ต่อให้ DB รั่ว ก็เอาแถวในตารางนี้ไปสร้างลิงก์รีเซ็ตที่ใช้ได้จริงไม่ได้
//
// ข้อมูลไหลเข้า: AuthController.ForgotPassword สุ่ม token → ส่ง plain token ไปในอีเมล → เก็บแต่ hash ที่นี่
// ข้อมูลไหลออก: AuthController.ResetPassword เอา token จาก URL มา hash แล้วหาแถวที่ตรง + ยังไม่หมดอายุ + ยังไม่ถูกใช้
//
// UsedAt เป็น pointer (NULL = ยังไม่ถูกใช้) — พอ reset สำเร็จจะ set เป็นเวลาปัจจุบัน กันเอา token เดิมมาใช้ซ้ำ
type PasswordResetToken struct {
	ID        int        `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int        `gorm:"column:user_id;type:integer;not null;index:idx_password_reset_tokens_user" json:"user_id"`
	TokenHash string     `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at;type:timestamp" json:"used_at"`
	CreatedAt time.Time  `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "password_reset_tokens"
func (PasswordResetToken) TableName() string { return "password_reset_tokens" }
