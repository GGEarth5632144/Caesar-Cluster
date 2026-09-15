package entity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// สถานะของ service ระหว่างวงจรชีวิต
//
// "สร้าง object บนคลัสเตอร์สำเร็จ" ไม่เท่ากับ "workload รันอยู่จริง" — k8s รับ object แล้วตอบสำเร็จ
// ทันที ต่อให้ไม่มี node ว่างหรือ container จะตายทันทีที่ขึ้น สถานะจริงจึงมาจาก ServiceHealthMonitor
// ที่ไปถามคลัสเตอร์ ไม่ใช่จากผลลัพธ์ของ DeployService
const (
	ServiceCreating = "creating" // บันทึกลง DB แล้ว กำลังรอ provisioner สร้างของบนคลัสเตอร์

	// ServicePending = object ถูกสร้างแล้วแต่ยังไม่มี Pod รันอยู่จริง
	// สาเหตุที่เจอบ่อย: ไม่มี node ไหนเหลือทรัพยากรพอ (FailedScheduling) หรือกำลังดึง image อยู่
	ServicePending = "pending"

	ServiceRunning = "running" // มี Pod รันอยู่จริงและผ่าน readiness แล้ว

	// ServiceCrashLoop = container ขึ้นมาแล้วตายซ้ำๆ — เกือบทุกครั้งคือ env ไม่ครบหรือตั้งค่าผิด
	// แยกจาก failed เพราะผู้ใช้แก้เองได้ และของยังอยู่บนคลัสเตอร์ (ยังกินโควตาอยู่)
	ServiceCrashLoop = "crashloop"

	ServiceFailed = "failed" // provisioner สร้างของบนคลัสเตอร์ไม่สำเร็จตั้งแต่ต้น
)

// ServiceStatusSettled = สถานะที่ไม่เปลี่ยนเองอีกแล้ว — หน้าเว็บหยุดดึงซ้ำ monitor ลดความถี่ลง
func ServiceStatusSettled(status string) bool {
	return status == ServiceRunning || status == ServiceFailed
}

// ContainerPort = พอร์ตที่โปรเซสข้างใน container ฟังอยู่ (ตรงกับ EXPOSE ใน image) คนละชั้นกับ
// NodePort ที่ k8s จ่ายให้ (30000-32767) เป็นทางเข้าจากนอก — Service เป็นตัวเชื่อมสองอันนี้
//
// Replicas = จำนวน Pod ที่รันขนานกัน (1 Pod = 1 container) กินโควตา namespace เป็น cpu_milli × replicas
// เพดาน 10 กันตั้งเลขหลุดจนกินทั้งคลัสเตอร์
const (
	DefaultContainerPort = 8080
	MinContainerPort     = 1
	MaxContainerPort     = 65535

	DefaultReplicas = 1
	MinReplicas     = 1
	MaxReplicas     = 10
)

// DatabaseReplicas = จำนวน Pod ของ database ตรึงที่ 1 เสมอ ไม่ใช่ข้อจำกัดชั่วคราว
// สอง Pod ที่ mount PVC ก้อนเดียวกันแล้วต่างคนต่างเขียน คือข้อมูลพัง ไม่ใช่ HA
// (HA ของ database ต้องทำด้วย replication ของ engine เอง ซึ่งคนละเรื่องกับ replicas)
const DatabaseReplicas = 1

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
	ContainerPort     int       `gorm:"column:container_port;type:integer;not null;default:8080;check:container_port BETWEEN 1 AND 65535" json:"container_port"`
	NodePort          *int      `gorm:"column:node_port;type:integer;check:node_port IS NULL OR (node_port BETWEEN 30000 AND 32767)" json:"node_port"`
	Replicas          int       `gorm:"column:replicas;type:integer;not null;default:1;check:replicas BETWEEN 1 AND 10" json:"replicas"`
	Status            string    `gorm:"column:status;type:varchar(20);not null;default:creating" json:"status"`
	EnvVars           EnvVarMap `gorm:"column:env_vars;type:jsonb;not null;default:'{}'" json:"env_vars"`

	// ── สถานะจริงจากคลัสเตอร์ (ServiceHealthMonitor เป็นคนเขียน) ─────────────────────

	// StatusReason = รหัสสั้นๆ จากคลัสเตอร์ (CrashLoopBackOff, ImagePullBackOff, FailedScheduling)
	// หน้าเว็บใช้เลือกคำแนะนำที่ตรงกับสาเหตุ ไม่ใช่ขึ้นข้อความเดียวกันหมด
	StatusReason string `gorm:"column:status_reason;type:varchar(60);not null;default:''" json:"status_reason"`

	// StatusMessage = คำอธิบาย + log ท้ายๆ ก่อน container ตาย
	// เก็บลง DB เพราะ Pod ที่ crash loop ถูกสร้างใหม่เรื่อยๆ log รอบที่บอกสาเหตุจริงหายก่อนผู้ใช้เปิดดู
	StatusMessage string `gorm:"column:status_message;type:text;not null;default:''" json:"status_message"`

	// RestartCount = จำนวนครั้งที่ container ถูกสร้างใหม่ — แยก "ช้า" ออกจาก "ตายแล้วเกิดใหม่วนไป"
	RestartCount int `gorm:"column:restart_count;type:integer;not null;default:0" json:"restart_count"`

	// StatusCheckedAt = ครั้งล่าสุดที่ถามคลัสเตอร์สำเร็จ (nil = ยังไม่เคยถามได้เลย)
	StatusCheckedAt *time.Time `gorm:"column:status_checked_at;type:timestamp" json:"status_checked_at"`

	// IsDatabase = สวิตช์ที่ผู้ใช้กดตอนสร้าง คุม 4 อย่างพร้อมกันเพราะทั้งสี่ถูกหรือผิดพร้อมกันเสมอ:
	// ClusterIP + NetworkPolicy (เข้าได้เฉพาะใน namespace), PVC ตาม StorageMB/DataPath,
	// ตรึง 1 Pod ตาม DatabaseReplicas และใช้ StatefulSet แทน Deployment — RollingUpdate ของ
	// Deployment ปั้น Pod ใหม่ก่อนฆ่าตัวเก่า สองตัวจะแย่ง PVC ก้อนเดียวกันจน deploy ค้างถาวร
	IsDatabase bool `gorm:"column:is_database;type:boolean;not null;default:false" json:"is_database"`

	// DataPath = จุดที่ PVC ถูก mount ("" ถ้าไม่ใช่ database) — ผู้ใช้กรอกเอง ระบบไม่เดาให้
	// เดาผิดแล้ว deploy สำเร็จและดิสก์ถูกจอง แต่ image เขียนลงที่อื่น ข้อมูลหายตอน restart แบบเงียบๆ
	// ห้ามแก้หลัง deploy: ย้ายจุด mount = ข้อมูลเดิมหายไปจากสายตาโปรแกรมทั้งที่ยังอยู่บนดิสก์
	DataPath string `gorm:"column:data_path;type:varchar(200);not null;default:''" json:"data_path"`

	// StorageMB = ขนาด PVC (0 = ไม่ใช่ database) — เป็น MB จำนวนเต็มเสมอ
	// หน่วยที่ผู้ใช้เลือกบนหน้าจอเป็นเรื่องการแสดงผล ไม่ส่งขึ้นมาให้ต้องตรวจซ้ำทุกชั้น
	StorageMB int `gorm:"column:storage_mb;type:integer;not null;default:0;check:storage_mb >= 0" json:"storage_mb"`

	// DeleteAt = เวลาที่แอดมินตั้งให้ลบ (nil = ไม่ได้ตั้ง) — ServiceManager.StartScheduledDeletion เป็นคนลบ
	DeleteAt *time.Time `gorm:"column:delete_at;type:timestamp;index:idx_services_delete_at" json:"delete_at"`

	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "services"
func (Service) TableName() string { return "services" }
