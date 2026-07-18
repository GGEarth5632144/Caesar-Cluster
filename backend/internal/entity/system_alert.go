package entity

import "time"

// ระดับความรุนแรงของ system alert
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// SystemAlert = ตาราง system_alerts — แจ้งเตือนระดับระบบที่ admin ต้องดู (ไม่ผูกกับ user คนใดคนหนึ่ง)
//
// ข้อมูลไหลเข้า: monitoring/health-check ต่างๆ (เช่น IPCMonitor เจอเครื่อง down, Provisioner deploy พัง) สร้างแจ้งเตือนเข้ามา
// source_type/source_name บอกว่าแจ้งเตือนมาจากอะไร (เช่น source_type="ipc_monitor", source_name=mac_id ของเครื่องที่ down)
// ข้อมูลไหลออก: AdminController โชว์หน้าแจ้งเตือนระบบ, is_read ใช้ mark ว่า admin เปิดอ่านแล้วหรือยัง
type SystemAlert struct {
	ID          int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Severity    string    `gorm:"column:severity;type:varchar(10);not null;check:severity IN ('critical','warning','info')" json:"severity"`
	SourceType  string    `gorm:"column:source_type;type:varchar(50);not null" json:"source_type"`
	SourceName  string    `gorm:"column:source_name;type:varchar(100);not null" json:"source_name"`
	StatusTitle string    `gorm:"column:status_title;type:varchar(100);not null" json:"status_title"`
	Detail      string    `gorm:"column:detail;type:text" json:"detail"`
	IsRead      bool      `gorm:"column:is_read;type:boolean;not null;default:false" json:"is_read"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "system_alerts"
func (SystemAlert) TableName() string { return "system_alerts" }
