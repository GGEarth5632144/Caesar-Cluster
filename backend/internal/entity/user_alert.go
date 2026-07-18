package entity

import "time"

// UserAlert = ตาราง user_alerts — แจ้งเตือนที่ส่งถึง user รายคน (เช่น โควตาใกล้เต็ม, container ล่ม, request ถูกอนุมัติ)
//
// ข้อมูลไหลเข้า: ระบบ (QuotaService / IPCMonitor sync / RequestController) สร้างแจ้งเตือนเมื่อเจอเหตุการณ์ที่เกี่ยวกับ user คนนั้นโดยตรง
// ข้อมูลไหลออก: user เปิดกระดิ่งแจ้งเตือนของตัวเอง อ่านแถวที่ user_id ตรงกับตัวเอง
type UserAlert struct {
	ID        int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	UserID    int       `gorm:"column:user_id;type:integer;not null;index:idx_user_alerts_user" json:"user_id"`
	Title     string    `gorm:"column:title;type:varchar(100);not null" json:"title"`
	Message   string    `gorm:"column:message;type:text;not null" json:"message"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "user_alerts"
func (UserAlert) TableName() string { return "user_alerts" }
