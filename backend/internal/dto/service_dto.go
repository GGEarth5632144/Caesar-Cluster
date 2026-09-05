package dto

// CreateServiceRequest = body ของ POST /api/services — ขอ deploy workload เข้า namespace ของตัวเอง
//
// เลือกสเปกได้ 2 ทาง:
//  1. ส่ง request_template_id (เลือกจาก "choices" ที่ admin สร้างไว้) → ระบบใช้ cpu/ram ของ template นั้น
//  2. ไม่ส่ง request_template_id แต่กรอก cpu_milli / ram_mb เองตามต้องการ
//
// เพดานใน binding เป็นเพดานของ service 1 ตัว (300% = 3000m, 2 GB)
// ส่วนโควตารวมทั้ง namespace ถูกเช็คอีกชั้นใน QuotaService (binding ตรงนี้ไม่รู้จักโควตา)
// data flow: JSON จาก client → ServiceController.Create → services.CreateServiceParams → ServiceManager.Create
type CreateServiceRequest struct {
	Name              string `json:"name" binding:"required,min=3,max=50"`
	Image             string `json:"image" binding:"required,min=3,max=200"`
	RequestTemplateID *int   `json:"request_template_id" binding:"omitempty,min=1"`
	CPUMilli          int    `json:"cpu_milli" binding:"required_without=RequestTemplateID,omitempty,min=100,max=3000"`
	RAMMB             int    `json:"ram_mb" binding:"required_without=RequestTemplateID,omitempty,min=128,max=2048"`
	// ContainerPort ไม่ใช่ NodePort — คนละชั้นกัน (ดู entity/service.go)
	// ทั้งคู่ omitempty: ไม่ส่งมา = ServiceManager เติม default ให้ (8080 / 1 replica)
	ContainerPort int `json:"container_port" binding:"omitempty,min=1,max=65535"`
	// หักโควตา namespace เป็น cpu_milli × replicas (เช็คใน QuotaService)
	Replicas int `json:"replicas" binding:"omitempty,min=1,max=10"`
	// EnvVars ไม่บังคับ — เช็ครูปแบบ/จำนวนแยกใน controller (isValidEnvVars) แทนการใช้ binding tag
	// เพราะ go-playground/validator เช็ค map ได้จำกัด (ไม่มี built-in ตรวจ key pattern ของแต่ละ entry)
	EnvVars map[string]string `json:"env_vars"`
}

// ScaleServiceRequest = body ของ PATCH /api/services/:id/scale — ปรับจำนวน Pod ของ service ที่ deploy แล้ว
// data flow: ServiceController.Scale → ServiceManager.Scale → QuotaService.ReserveScale (เช็คโควตาก่อน UPDATE)
type ScaleServiceRequest struct {
	Replicas int `json:"replicas" binding:"required,min=1,max=10"`
}
