package entity

import "time"

// UserContainer = ตาราง user_containers — ให้ user monitor container ของตัวเองได้ (ดูอย่างเดียว ใช้ terminal ไม่ได้)
// ตามที่ ERD กำกับไว้ว่า "ใช้ terminal ไม่ได้ ได้แค่ monitoring"
//
// ข้อมูลไหลเข้า: agent บนเครื่อง IPC (ipc_monitors) sync สถานะ container ของแต่ละ namespace เข้ามาเป็นระยะ
// ข้อมูลไหลออก: user เปิดหน้า monitoring ของตัวเอง อ่านแถวที่ namespace_id ตรงกับ namespace ของตัวเอง,
// admin อ่านได้ทุกแถวเหมือนกัน (แยกสิทธิ์ที่ชั้น controller ไม่ใช่ที่ตารางนี้)
type UserContainer struct {
	ID            int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	NamespaceID   int       `gorm:"column:namespace_id;type:integer;not null;index:idx_user_containers_namespace" json:"namespace_id"`
	IPCID         int       `gorm:"column:ipc_id;type:integer;not null;index:idx_user_containers_ipc" json:"ipc_id"`
	ContainerName string    `gorm:"column:container_name;type:varchar(100);not null" json:"container_name"`
	RunningType   string    `gorm:"column:running_type;type:varchar(20);not null" json:"running_type"`
	UsedRAM       int       `gorm:"column:used_ram;type:integer;not null;default:0" json:"used_ram"`
	UsedCPU       int       `gorm:"column:used_cpu;type:integer;not null;default:0" json:"used_cpu"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "user_containers"
func (UserContainer) TableName() string { return "user_containers" }
