package entity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// สถานะของ service ระหว่างวงจรชีวิต
const (
	ServiceCreating = "creating" // บันทึกลง DB แล้ว กำลังรอ provisioner deploy จริง
	ServiceRunning  = "running"  // deploy ขึ้น k8s สำเร็จ
	ServiceFailed   = "failed"   // provisioner deploy ไม่สำเร็จ
)

// EnvVarMap คือ environment variables ของ service เดียว เก็บเป็น jsonb คอลัมน์เดียว
// ตั้งใจใช้ map[string]string (ไม่ใช้ JSONB ที่มีอยู่แล้วซึ่งเป็น map[string]any) เพราะ env var
// เป็น key-value string ล้วนเสมอ — ฝั่งที่ใช้งานจริง (provisioner) จะได้ไม่ต้อง type-assert ทุกค่า
// เขียนเองด้วย stdlib ล้วน (database/sql/driver + encoding/json) ตามแบบเดียวกับ JSONB ใน jsonb.go
// ไม่ต้องเพิ่ม dependency gorm.io/datatypes
type EnvVarMap map[string]string

// Value แปลง map ในหน่วยความจำ → bytes ก่อนเขียนลง DB (ฝั่ง "ส่งออก")
// map ว่าง/nil เก็บเป็น "{}" แทน NULL เพื่อให้อ่านกลับมาเป็น map ว่างเสมอ (ไม่ต้อง nil-check ฝั่งอ่าน)
func (e EnvVarMap) Value() (driver.Value, error) {
	if len(e) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(e)
	return string(b), err
}

// Scan แปลง bytes จาก DB → map ในหน่วยความจำ (ฝั่ง "รับเข้า")
func (e *EnvVarMap) Scan(value any) error {
	if value == nil {
		*e = EnvVarMap{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("entity: EnvVarMap.Scan ต้องการ []byte หรือ string, ได้ %T", value)
	}
	if len(b) == 0 {
		*e = EnvVarMap{}
		return nil
	}
	return json.Unmarshal(b, e)
}

// Service = ตาราง services — workload (container) 1 ตัวที่ผู้ใช้ deploy เข้าไปใน namespace ของตัวเอง
// (มาแทน entity VM เดิม เพราะเราไป Kubernetes ไม่ใช่ Proxmox แล้ว)
//
// ข้อมูลไหลเข้า: ServiceController.Create → QuotaService เช็คโควตาของ namespace → INSERT ภายใน transaction
// ข้อมูลไหลออก: ServiceManager.ListByNamespace อ่านไปโชว์, QuotaService SUM cpu_milli/ram_mb
// ของทุก service ใน namespace เพื่อคิดว่าโควตาเหลือเท่าไหร่
//
// CPUMilli/RAMMB เป็นค่า snapshot ที่ก๊อปมาจาก RequestTemplate ตอนสร้าง (ดูเหตุผลใน request_template.go)
// RequestTemplateID เก็บไว้อ้างอิงเฉยๆ ว่ามาจาก choice ไหน (เป็น pointer เพราะ user กรอกสเปกเองโดยไม่เลือก template ก็ได้)
//
// ContainerPort คือ port ที่แอปข้างใน container ฟังอยู่ (ปลายทางที่ NodePort จะวิ่งเข้าไปหา)
// เป็น pointer เพราะไม่บังคับกรอก — ถ้าไม่ระบุ provisioner จะเดาจาก env var ชื่อ PORT/CONTAINER_PORT
// ก่อน แล้วค่อยตกไปที่ค่า default ของคลัสเตอร์ (K8S_DEFAULT_CONTAINER_PORT)
//
// NodePort คือช่องทางที่ user ใช้เข้าถึง service ของตัวเอง — เปิดเป็น k8s Service ชนิด NodePort
// (ทุก node อยู่ subnet เดียวกัน ไม่มี cloud LoadBalancer ให้ใช้ เลยเลือกแบบนี้แทน Ingress)
// user ต่อเข้าที่ <node-ip ตัวไหนก็ได้>:<node_port> — เป็น pointer เพราะยังไม่มีค่าจนกว่า provisioner จะ deploy สำเร็จ
type Service struct {
	ID                int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	NamespaceID       int       `gorm:"column:namespace_id;type:integer;not null;uniqueIndex:uni_services_ns_name" json:"namespace_id"`
	Name              string    `gorm:"column:name;type:varchar(50);not null;uniqueIndex:uni_services_ns_name" json:"name"`
	CreatedBy         int       `gorm:"column:created_by;type:integer;not null;index:idx_services_creator" json:"created_by"`
	RequestTemplateID *int      `gorm:"column:request_template_id;type:integer" json:"request_template_id"`
	Image             string    `gorm:"column:image;type:varchar(200);not null" json:"image"`
	CPUMilli          int       `gorm:"column:cpu_milli;type:integer;not null;check:cpu_milli > 0" json:"cpu_milli"`
	RAMMB             int       `gorm:"column:ram_mb;type:integer;not null;check:ram_mb > 0" json:"ram_mb"`
	ContainerPort     *int      `gorm:"column:container_port;type:integer;check:container_port IS NULL OR (container_port BETWEEN 1 AND 65535)" json:"container_port"`
	NodePort          *int      `gorm:"column:node_port;type:integer;check:node_port IS NULL OR (node_port BETWEEN 30000 AND 32767)" json:"node_port"`
	Status            string    `gorm:"column:status;type:varchar(20);not null;default:creating" json:"status"`
	EnvVars           EnvVarMap `gorm:"column:env_vars;type:jsonb;not null;default:'{}'" json:"env_vars"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "services"
func (Service) TableName() string { return "services" }
