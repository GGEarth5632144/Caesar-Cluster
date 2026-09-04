package entity

import "time"

// EmailVerification = ตาราง email_verifications — ลิงก์ยืนยันอีเมลแบบใช้ได้ครั้งเดียว
// เก็บแต่ sha256 ของ token ไม่เก็บตัวจริง (หลักการเดียวกับ PasswordResetToken)
//
// ข้อมูลไหลเข้า: AuthController.sendVerificationLink — ออกใบใหม่ทุกครั้งที่ต้องส่งลิงก์
// ข้อมูลไหลออก: AuthController.VerifyEmail — hash token จาก URL → หาแถวที่ตรง → เปิดใช้งานบัญชี
type EmailVerification struct {
	ID        int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int       `gorm:"column:user_id;type:integer;not null;index:idx_email_verifications_user" json:"user_id"`
	TokenHash string    `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	// NULL = ยังไม่ถูกใช้ และเป็นสิ่งเดียวที่ทำให้ "ใช้ได้ครั้งเดียว" เป็นจริง: กดลิงก์สำเร็จออก JWT ให้เลย
	// ถ้าไม่ปิดใบทิ้ง ลิงก์ที่ค้างในกล่องอีเมลจะกลายเป็นกุญแจเข้าบัญชีที่ใช้ซ้ำได้ไม่จำกัด
	UsedAt *time.Time `gorm:"column:used_at;type:timestamp" json:"used_at"`
	// ทำหน้าที่สองอย่าง: อายุของแถว และ "ส่งเมลรอบล่าสุดเมื่อไร" ที่ใช้คุม cooldown
	// ไม่มี last_sent_at แยก เพราะส่งลิงก์ใหม่ = แถวใหม่เสมอ สองค่านั้นจึงเท่ากันตลอด
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "email_verifications"
func (EmailVerification) TableName() string { return "email_verifications" }
