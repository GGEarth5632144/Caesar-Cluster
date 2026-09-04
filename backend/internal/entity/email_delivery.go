package entity

import "time"

// สถานะการส่ง: SMTP ตอบรับ (250) หรือไม่ — SMTP บอกได้แค่นี้ ไม่รู้ว่าถึงกล่องขาเข้าไหม
const (
	EmailStatusSent   = "sent"
	EmailStatusFailed = "failed"
)

// EmailDelivery = ตาราง email_deliveries — บันทึกผลการส่งอีเมลทุกฉบับ (เขียนโดย mailer ผ่าน Recorder)
// ไว้ตอบว่า "ส่งจริงไหม กี่โมง พังตรงไหน" ไม่มี FK ไป users (เหตุผลที่ config.addForeignKeys)
type EmailDelivery struct {
	ID int `gorm:"column:id;type:serial;primaryKey" json:"id"`
	// pointer เพราะบางเส้นทางส่งเมลโดยยังไม่รู้ว่าตรงกับ user ไหน
	UserID    *int   `gorm:"column:user_id;type:integer;index:idx_email_deliveries_user" json:"user_id"`
	ToEmail   string `gorm:"column:to_email;type:varchar(255);not null" json:"to_email"`
	Purpose   string `gorm:"column:purpose;type:varchar(30);not null;index:idx_email_deliveries_purpose" json:"purpose"`
	MessageID string `gorm:"column:message_id;type:varchar(255);not null" json:"message_id"`
	Status    string `gorm:"column:status;type:varchar(10);not null;index:idx_email_deliveries_status" json:"status"`
	// error จาก SMTP ตอน failed
	Error string `gorm:"column:error;type:text" json:"error"`
	// เวลาที่ใช้คุยกับ SMTP (ms)
	DurationMS int       `gorm:"column:duration_ms;type:integer;not null;default:0" json:"duration_ms"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamp;not null;default:now();index:idx_email_deliveries_created" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "email_deliveries"
func (EmailDelivery) TableName() string { return "email_deliveries" }
