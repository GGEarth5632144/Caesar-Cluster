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
}
