package entity

import "time"

// TrustedDevice = ตาราง trusted_devices — เครื่องที่ผ่าน OTP มาแล้ว จึงข้ามการกรอก OTP ได้ชั่วคราว
//
// ความเชื่อใจต้องผูกกับ "เครื่อง" ไม่ใช่ "คน": ถ้าจำแค่ว่า user คนนี้เคยผ่าน OTP แล้ว
// (เช่นเก็บเป็น timestamp ในตาราง users) คนที่ขโมยรหัสผ่านไปก็ล็อกอินจากเครื่องตัวเองได้เลย
// โดยไม่ต้องมีอีเมล ซึ่งทำให้ OTP ไม่เหลือความหมาย
//
// ข้อมูลไหลเข้า: AuthController.VerifyOTP — กรอกรหัสถูกแล้วออก device token ให้ client เก็บ
// ข้อมูลไหลออก: AuthController.Login — client แนบ device_token มา ถ้ายังไม่หมดอายุก็ออก JWT เลย
//
// เก็บแค่ hash ของ token เหมือน PasswordResetToken — DB รั่วแล้วสวมรอยเป็นเครื่องที่เชื่อใจไม่ได้
type TrustedDevice struct {
	ID        int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int       `gorm:"column:user_id;type:integer;not null;index:idx_trusted_devices_user" json:"user_id"`
	TokenHash string    `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time `gorm:"column:expires_at;type:timestamp;not null" json:"expires_at"`
	// ใช้ข้าม OTP ครั้งล่าสุดเมื่อไร — ไม่ได้ใช้ตัดสินใจอะไร เก็บไว้ให้ไล่ดูย้อนหลังเผื่อมีเรื่องต้องสืบ
	LastUsedAt time.Time `gorm:"column:last_used_at;type:timestamp;not null" json:"last_used_at"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "trusted_devices"
func (TrustedDevice) TableName() string { return "trusted_devices" }
