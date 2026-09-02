package entity

import "time"

// OTPChallenge = ตาราง otp_challenges — ใบยืนยันตัวตนหนึ่งใบต่อการล็อกอินหนึ่งครั้งที่ต้องกรอก OTP
//
// ตอน login สำเร็จ (รหัสผ่านถูก) แต่ยังไม่ผ่าน OTP ระบบจะ "ยังไม่ออก JWT" แต่สร้างแถวนี้แทน
// แล้วส่งรหัส 6 หลักไปทางอีเมล — client ถือ challenge token ไว้แล้วเอามาแลก JWT ที่ /api/verify-otp
//
// ข้อมูลไหลเข้า: AuthController.Login (สร้างใบใหม่) / ResendOTP (ออกรหัสใหม่ให้ใบเดิม)
// ข้อมูลไหลออก: AuthController.VerifyOTP หา challenge จาก token → เทียบรหัส → ออก JWT + trusted device
//
// เก็บ hash ของทั้งสองค่า ไม่เก็บตัวจริง (หลักการเดียวกับ PasswordResetToken):
//   - TokenHash = sha256 ของ challenge token (สุ่ม 32 ไบต์ เดาไม่ได้อยู่แล้ว sha256 พอ)
//   - CodeHash  = bcrypt ของรหัส 6 หลัก ต้องใช้ bcrypt ไม่ใช่ sha256 เพราะรหัส 6 หลักมีแค่ล้านค่า
//     ถ้า DB รั่วแล้วเป็น sha256 จะไล่ทุกค่าเจอในเสี้ยววินาที
//
// Remember เก็บไว้ตั้งแต่ตอน login เพราะ JWT ออกจริงตอน verify — ถ้าไม่เก็บก็ต้องให้ client
// ส่งมาใหม่ตอน verify ซึ่งแปลว่าใครก็ยืดอายุ token ตัวเองเป็น 30 วันได้ด้วยการแก้ request
type OTPChallenge struct {
	ID        int    `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int    `gorm:"column:user_id;type:integer;not null;index:idx_otp_challenges_user" json:"user_id"`
	TokenHash string `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	CodeHash  string `gorm:"column:code_hash;type:varchar(255);not null" json:"-"`
	// Attempts = กรอกรหัสผิดไปกี่ครั้งแล้วสำหรับใบนี้ — ถึงเพดานเมื่อไรใบนี้ตายทันที
	// กันไล่เดารหัส 6 หลักด้วยการยิงซ้ำๆ (ล้านค่าเดาได้ในไม่กี่นาทีถ้าไม่จำกัด)
	Attempts   int        `gorm:"column:attempts;type:integer;not null;default:0" json:"attempts"`
	Remember   bool       `gorm:"column:remember;type:boolean;not null;default:false" json:"remember"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	ConsumedAt *time.Time `gorm:"column:consumed_at;type:timestamp" json:"consumed_at"`
	// LastSentAt = ส่งอีเมลรอบล่าสุดเมื่อไร ใช้คุมปุ่ม "ส่งรหัสใหม่" ไม่ให้กดรัวๆ
	LastSentAt time.Time `gorm:"column:last_sent_at;type:timestamp;not null" json:"last_sent_at"`
	// Resends = กด "ส่งรหัสใหม่" ไปกี่ครั้งแล้วสำหรับใบนี้ — มีเพดานเพราะลำพัง cooldown 60 วิ
	// กันได้แค่การกดรัว แต่ไม่กันคนที่ยอมรอทีละนาทีเพื่อถล่มอีเมลของเหยื่อไปเรื่อยๆ
	Resends   int       `gorm:"column:resends;type:integer;not null;default:0" json:"resends"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "otp_challenges"
func (OTPChallenge) TableName() string { return "otp_challenges" }
