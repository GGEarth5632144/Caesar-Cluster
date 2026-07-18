package dto

// CreateRequestTemplateRequest = body ของ POST /api/admin/request-templates — admin สร้าง "choice" ให้ผู้ใช้เลือก
// เพดานตรงกับเพดานของ service 1 ตัว (3000m / 2048MB) เพราะ template ก็คือสเปกของ service ตัวหนึ่งได้เหมือนกัน
// data flow: JSON จาก client → AdminController.CreateRequestTemplate → INSERT request_templates
type CreateRequestTemplateRequest struct {
	Name          string `json:"name" binding:"required,min=2,max=50"`
	CPULimitMilli int    `json:"cpu_limit_milli" binding:"required,min=100,max=3000"`
	RAMLimitMB    int    `json:"ram_limit_mb" binding:"required,min=128,max=2048"`
}
