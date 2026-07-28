package dto

// CreateAIReviewRequestRequest = body ของ POST /api/ai-review-requests
//
// RequestQuotar.tsx (handleDeployWithAI) ยิงมาที่นี่คู่ขนานกับตอนที่ยิง POST /api/review ไปที่ Cluster-AI
// โดยตรง — เก็บไว้เป็น "ใบเสร็จ" ให้ AIReviewPage.tsx ดึงกลับมาได้ถ้า router state หาย (refresh/เปิดลิงก์ตรง)
// เพราะ Cluster-AI เก็บแค่สถานะ pipeline ใน memory ไม่ได้เก็บ service_name/image/cpu/ram ที่ submit มาด้วย
//
// cpu_milli/ram_mb เป็นค่าที่ frontend resolve มาแล้ว (จาก template หรือกรอกเอง) ไม่ใช่ conditional
// แบบ CreateServiceRequest เพราะจุดนี้แค่บันทึกไว้เฉยๆ ไม่ได้ผูกกับ QuotaService
type CreateAIReviewRequestRequest struct {
	RequestID         string `json:"request_id" binding:"required,min=1,max=64"`
	ServiceName       string `json:"service_name" binding:"required,min=3,max=50"`
	Image             string `json:"image" binding:"required,min=3,max=200"`
	RequestTemplateID *int   `json:"request_template_id" binding:"omitempty,min=1"`
	CPUMilli          int    `json:"cpu_milli" binding:"required,min=100,max=3000"`
	RAMMB             int    `json:"ram_mb" binding:"required,min=128,max=2048"`
	// EnvVars ไม่บังคับ — เช็ครูปแบบ/จำนวนแยกใน controller (isValidEnvVars) เหมือน CreateServiceRequest
	EnvVars map[string]string `json:"env_vars"`
}
