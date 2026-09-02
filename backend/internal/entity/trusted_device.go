package entity

import "time"

// TrustedDevice = ตาราง trusted_devices — เครื่องที่ผ่าน OTP มาแล้ว จึงข้ามการกรอก OTP ได้ชั่วคราว
//
// ถ้าไม่มีตารางนี้ ผู้ใช้จะต้องเปิดอีเมลกรอกรหัสทุกครั้งที่ล็อกอิน ซึ่งน่ารำคาญจนคนเลิกใช้
// แต่ถ้าจำแค่ว่า "user คนนี้เคยผ่าน OTP แล้ว" (เก็บเป็น timestamp ที่ตาราง users) จะเสียจุดประสงค์
// ทั้งหมดของ OTP ไป เพราะคนที่ขโมยรหัสผ่านไปก็ล็อกอินจากเครื่องตัวเองได้เลยโดยไม่ต้องมีอีเมล
// เลยต้องผูกความเชื่อใจไว้กับ "เครื่อง" ไม่ใช่ "คน"
//
// ข้อมูลไหลเข้า: AuthController.VerifyOTP — กรอกรหัสถูกแล้วออก device token ให้ client เก็บไว้
// ข้อมูลไหลออก: AuthController.Login — client แนบ device_token มาด้วย ถ้ายังไม่หมดอายุก็ออก JWT เลย
//
// เก็บแค่ hash ของ token เหมือน PasswordResetToken — DB รั่วแล้วเอาไปสวมรอยเป็นเครื่องที่เชื่อใจไม่ได้
type TrustedDevice struct {
	ID        int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int       `gorm:"column:user_id;type:integer;not null;index:idx_trusted_devices_user" json:"user_id"`
	TokenHash string    `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	// LastUsedAt = ใช้ข้าม OTP ครั้งล่าสุดเมื่อไร — ไม่ได้ใช้ตัดสินใจอะไร เก็บไว้ให้ไล่ดูย้อนหลังได้
	// ว่าเครื่องที่เชื่อใจอยู่ยังถูกใช้งานจริงไหม (เผื่อมีเรื่องต้องสืบ)
	LastUsedAt time.Time `gorm:"column:last_used_at;type:timestamp;not null" json:"last_used_at"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "trusted_devices"
func (TrustedDevice) TableName() string { return "trusted_devices" }
