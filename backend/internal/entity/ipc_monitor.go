package entity

import "time"

// สถานะของเครื่อง IPC — ตาม Scrape_Data_Prometheus ล่าสุดที่ได้มา
const (
	IPCStatusActive = "active" // scrape ได้ปกติ
	IPCStatusDown   = "down"   // scrape ไม่ตอบ/ขาดการเชื่อมต่อ
)

// IPCMonitor = ตาราง ipc_monitors (คือ ipc_mornitor ใน ERD)
// ให้ admin monitor เครื่อง IPC ทั้งหมด (ตามแผน 40 ตัว) ที่รัน container ของผู้ใช้อยู่
//
// ตัดฟิลด์ container_count ออกจาก ERD ตั้งต้น — เก็บ column แยกซ้ำจะ sync ไม่ตรงกับของจริงได้
// (หลักการเดียวกับที่ user.go ใช้กับ member_count ของ namespace) ให้นับสดจาก
// COUNT(user_containers WHERE ipc_id = ?) ตอน query แทน
//
// ข้อมูลไหลเข้า: Prometheus scrape เครื่องแต่ละตัวเป็นระยะ แล้ว exporter/collector เขียนผลกลับเข้ามาที่แถวนี้
// (status/scrape_data_prometheus ถูก sync ทับของเดิมทุกรอบ)
// ข้อมูลไหลออก: AdminController อ่านไปโชว์หน้า monitoring รวมของทุกเครื่อง (เห็นได้เฉพาะ admin ตาม ERD)
// UserContainer.IPCID อ้างกลับมาที่ตารางนี้ เพื่อรู้ว่า container ของ user รันอยู่บนเครื่องไหน
type IPCMonitor struct {
	ID                   int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	MacID                string    `gorm:"column:mac_id;type:varchar(50);unique;not null" json:"mac_id"`
	IPAddress            string    `gorm:"column:ip_address;type:varchar(45);not null" json:"ip_address"`
	Status               string    `gorm:"column:status;type:varchar(10);not null;default:active;check:status IN ('active','down')" json:"status"`
	ScrapeDataPrometheus JSONB     `gorm:"column:scrape_data_prometheus;type:jsonb" json:"scrape_data_prometheus,omitempty"`
	CreatedAt            time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "ipc_monitors"
func (IPCMonitor) TableName() string { return "ipc_monitors" }
