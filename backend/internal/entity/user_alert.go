package entity

import "time"

// ที่มาของ UserAlert — เก็บใน source_type เพื่อให้หน้าเว็บแยกไอคอน/ปลายทางของปุ่ม "ดูรายละเอียด" ได้
const (
	AlertSourceServiceLog = "service_log" // ตัวสแกน log เจอบรรทัดที่เป็น error (LogAlertScanner)
	AlertSourceSystem     = "system"      // ระบบสร้างเอง เช่น deploy ไม่สำเร็จ / โควตาใกล้เต็ม
)

// UserAlert = ตาราง user_alerts — แจ้งเตือนที่ส่งถึง user รายคน (เช่น service ของกลุ่มพ่น error, โควตาใกล้เต็ม)
//
// ข้อมูลไหลเข้า: LogAlertScanner เดินสแกน log ของทุก service ที่ running อยู่เป็นรอบๆ เจอบรรทัดที่เข้าข่าย
// error แล้วเรียก AlertManager.Raise สร้างแถวให้สมาชิกทุกคนใน namespace นั้น
// ข้อมูลไหลออก: AlertController /api/alerts (หน้า Alerts) และ /api/alerts/unread-count (ตัวเลขวงกลมแดงบน Sidebar)
//
// ทำไมต้องมี Fingerprint + Count แทนการ INSERT ทุกบรรทัด: container ที่พังจริงพ่น error บรรทัดเดิม
// ซ้ำวินาทีละหลายรอบ ถ้า INSERT ทุกบรรทัด หน้า Alerts จะเป็นกำแพงข้อความเดียวกันหมื่นแถวจนหา
// เรื่องอื่นไม่เจอ เลยยุบข้อความที่เป็นเรื่องเดียวกันให้เหลือแถวเดียวแล้วนับ Count ขึ้นแทน
// (แบบเดียวกับ Sentry) — ดู alertFingerprint ใน services/log_alert_scanner.go
//
// ไม่ใส่ unique index บน fingerprint โดยตั้งใจ: ต้องยุบ "เฉพาะในหน้าต่างเวลาหนึ่ง" ด้วย
// error เดิมที่กลับมาหลังเงียบไปสามวันควรเป็นแจ้งเตือนใหม่ ไม่ใช่ไปบวก Count ของแถวที่อ่านไปแล้ว
// จึงทำที่ชั้นแอป (UPDATE ก่อน ไม่โดนค่อย INSERT)
type UserAlert struct {
	ID     int `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID int `gorm:"column:user_id;type:integer;not null;index:idx_user_alerts_user" json:"user_id"`

	Severity string `gorm:"column:severity;type:varchar(10);not null;default:info" json:"severity"`
	Title    string `gorm:"column:title;type:varchar(100);not null" json:"title"`
	Message  string `gorm:"column:message;type:text;not null" json:"message"`

	// SourceType/SourceName บอกว่าแจ้งเตือนนี้มาจากไหน (เช่น "service_log" + ชื่อ service)
	SourceType string `gorm:"column:source_type;type:varchar(50);not null;default:system" json:"source_type"`
	SourceName string `gorm:"column:source_name;type:varchar(100);not null;default:''" json:"source_name"`

	// ServiceID ผูกกับ service ต้นเรื่อง (ถ้ามี) — หน้าเว็บใช้ทำปุ่ม "ดู log" ลิงก์ตรงไปหน้า log viewer
	// เป็น pointer เพราะแจ้งเตือนบางประเภทไม่ได้มาจาก service ตัวใดตัวหนึ่ง
	ServiceID *int `gorm:"column:service_id;type:integer;index:idx_user_alerts_service" json:"service_id"`

	Fingerprint string `gorm:"column:fingerprint;type:varchar(64);not null;default:'';index:idx_user_alerts_fingerprint" json:"-"`
	Count       int    `gorm:"column:count;type:integer;not null;default:1" json:"count"`

	IsRead     bool      `gorm:"column:is_read;type:boolean;not null;default:false;index:idx_user_alerts_unread" json:"is_read"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
	LastSeenAt time.Time `gorm:"column:last_seen_at;type:timestamp;not null;default:now()" json:"last_seen_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "user_alerts"
func (UserAlert) TableName() string { return "user_alerts" }
