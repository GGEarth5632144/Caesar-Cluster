package dto

// CreateRequestRequest = body ของ POST /api/requests
// namespace_name เก็บ "ชนิด" (solo/group) ไม่ใช่ชื่อจริง — ชื่อจริงถูกเจนตอน admin approve
// data flow: JSON จาก client → RequestController.Create → INSERT requests (status = pending)
type CreateRequestRequest struct {
	Description   string `json:"description"`
	NamespaceName string `json:"namespace_name" binding:"required,oneof=solo group"`
	CPULimitMilli int    `json:"cpu_limit_milli" binding:"required,gt=0"`
	RAMLimitMB    int    `json:"ram_limit_mb" binding:"required,gt=0"`
}
