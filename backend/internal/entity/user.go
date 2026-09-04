package entity

import "time"

// User = ตาราง users — บัญชีผู้ใช้ (นักศึกษา + admin)
// NamespaceID เป็น pointer เพราะคนที่เพิ่งสมัครยังไม่มี space (NULL) ตามกติกา "1 คน = 1 space"
//
// student_id ใช้ tag `unique` ไม่ใช่ `uniqueIndex` โดยตั้งใจ — uniqueIndex ทำให้ AutoMigrate
// สั่ง DROP CONSTRAINT ชื่อที่มันเดาเองซึ่งไม่มีจริง แล้ว migrate พัง
type User struct {
	ID          int    `gorm:"column:id;type:serial;primaryKey" json:"id"`
	StudentID   string `gorm:"column:student_id;type:varchar(20);unique;not null" json:"student_id"`
	RoleID      int    `gorm:"column:role_id;type:integer;not null;index:idx_users_role" json:"role_id"`
	RealName    string `gorm:"column:real_name;type:varchar(100);not null" json:"real_name"`
	NickName    string `gorm:"column:nick_name;type:varchar(50)" json:"nick_name"`
	NamespaceID *int   `gorm:"column:namespace_id;type:integer;index:idx_users_namespace" json:"namespace_id"`
	Password    string `gorm:"column:password;type:varchar(255);not null" json:"-"`
	Gmail       string `gorm:"column:gmail;type:varchar(100);unique;not null" json:"gmail"`
	// เวลาที่กดลิงก์ยืนยันในอีเมล — NULL = ยังไม่ยืนยัน ล็อกอินไม่ได้ (ดู AuthController.Login)
	// เก็บเป็นเวลาไม่ใช่ bool เพราะ "ยืนยันเมื่อไร" ต้องใช้ตอนสืบย้อนหลัง ส่วน bool คำนวณจากมันได้อยู่แล้ว
	GmailVerifiedAt *time.Time `gorm:"column:gmail_verified_at;type:timestamp" json:"gmail_verified_at"`
	// ปีที่เข้าศึกษา (พ.ศ.) แกะจาก prefix ของ student_id ครั้งเดียวตอนสมัคร — คนละเรื่องกับ "ชั้นปี"
	// ที่ต้องคำนวณสดทุกครั้ง (ดู entity.YearLevel)
	EntryYear int       `gorm:"column:year;type:integer;not null;default:0" json:"year"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "users"
func (User) TableName() string { return "users" }

// GmailVerified รวมการเช็ค nil ไว้ที่เดียว ที่เรียกใช้จะได้ไม่ต้องจำว่า NULL แปลว่ายังไม่ยืนยัน
func (u User) GmailVerified() bool { return u.GmailVerifiedAt != nil }
