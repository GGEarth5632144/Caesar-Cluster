package dto

import "backend/internal/entity"

// CreateRequestRequest = body ของ POST /api/requests
// namespace_name เก็บ "ชนิด" (solo/group) ไม่ใช่ชื่อจริง — ชื่อจริงถูกเจนตอน admin approve
//
// RequestTemplateID ไม่บังคับ — ถ้าส่งมา (ผู้ใช้เลือก choice จากหน้า WorkspaceOnboarding)
// RequestController.Create จะก๊อป storage_gb ของ template นั้นมาเก็บเป็น snapshot ไว้ด้วย
// data flow: JSON จาก client → RequestController.Create → INSERT requests (status = pending)
type CreateRequestRequest struct {
	Description       string `json:"description"`
	NamespaceName     string `json:"namespace_name" binding:"required,oneof=solo group"`
	RequestTemplateID *int   `json:"request_template_id" binding:"omitempty,min=1"`
	CPULimitMilli     int    `json:"cpu_limit_milli" binding:"required,gt=0"`
	RAMLimitMB        int    `json:"ram_limit_mb" binding:"required,gt=0"`
}

// RequestWithRequester = entity.Request + ข้อมูลผู้ยื่นแบบย่อ ให้หน้า admin โชว์ชื่อ/รหัส นศ.
// แทนที่จะมีแค่ user_id เฉยๆ — enrich ทีเดียวตอน list แทนที่จะ join ใน query (ดู AdminController.ListAllRequests)
type RequestWithRequester struct {
	entity.Request
	RequesterName      string `json:"requester_name"`
	RequesterStudentID string `json:"requester_student_id"`
}
