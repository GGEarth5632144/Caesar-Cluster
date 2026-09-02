package entity

import "time"

// OTPChallenge = ตาราง otp_challenges — ใบยืนยันตัวตนหนึ่งใบต่อการล็อกอินหนึ่งครั้งที่ต้องกรอก OTP
//
// รหัสผ่านถูกแต่ยังไม่ผ่าน OTP = ยังไม่ออก JWT แต่สร้างแถวนี้แล้วส่งรหัส 6 หลักไปทางอีเมล
// client ถือ challenge token ไว้แล้วเอามาแลก JWT ที่ /api/verify-otp
//
// ข้อมูลไหลเข้า: AuthController.Login (ใบใหม่) / ResendOTP (รหัสใหม่ในใบเดิม)
// ข้อมูลไหลออก: AuthController.VerifyOTP → เทียบรหัส → ออก JWT + trusted device
//
// เก็บแต่ hash ไม่เก็บตัวจริง (หลักการเดียวกับ PasswordResetToken):
//   - TokenHash = sha256 ของ challenge token (สุ่ม 32 ไบต์ เดาไม่ได้อยู่แล้ว sha256 พอ)
//   - CodeHash  = bcrypt ของรหัส 6 หลัก ต้องเป็น bcrypt เพราะ 6 หลักมีแค่ล้านค่า
//     ถ้า DB รั่วแล้วเป็น sha256 จะไล่ครบทุกค่าได้ในเสี้ยววินาที
type OTPChallenge struct {
	ID        int    `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int    `gorm:"column:user_id;type:integer;not null;index:idx_otp_challenges_user" json:"user_id"`
	TokenHash string `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	CodeHash  string `gorm:"column:code_hash;type:varchar(255);not null" json:"-"`
	// กรอกผิดไปกี่ครั้งแล้ว — ถึง otpMaxAttempts เมื่อไรใบนี้ตายทันที กันไล่เดารหัสด้วยการยิงซ้ำๆ
	Attempts int `gorm:"column:attempts;type:integer;not null;default:0" json:"attempts"`
	// ติ๊ก "Remember For 30 Days" มาไหม เก็บไว้ตั้งแต่ตอน login เพราะ JWT ออกจริงตอน verify
	// ถ้าให้ client ส่งมาใหม่ตอน verify ก็เท่ากับใครก็ยืดอายุ token ตัวเองได้ด้วยการแก้ request
	Remember   bool       `gorm:"column:remember;type:boolean;not null;default:false" json:"remember"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	ConsumedAt *time.Time `gorm:"column:consumed_at;type:timestamp" json:"consumed_at"`
	// ส่งอีเมลรอบล่าสุดเมื่อไร — ใช้คุม cooldown ของปุ่ม "ส่งรหัสใหม่"
	LastSentAt time.Time `gorm:"column:last_sent_at;type:timestamp;not null" json:"last_sent_at"`
	// กดส่งรหัสใหม่ไปกี่ครั้งแล้ว — ต้องมีเพดานเพราะลำพัง cooldown 60 วิ กันได้แค่การกดรัว
	// ไม่กันคนที่ยอมรอทีละนาทีเพื่อถล่มอีเมลเหยื่อไปเรื่อยๆ
	Resends int `gorm:"column:resends;type:integer;not null;default:0" json:"resends"`
	// จุดตั้งต้นของอายุรวมของใบ (otpChallengeMaxLifetime) — ResendOTP ใช้คำนวณเพดาน expires_at
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "otp_challenges"
func (OTPChallenge) TableName() string { return "otp_challenges" }
