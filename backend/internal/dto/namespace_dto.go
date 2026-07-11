package dto

// CreateNamespaceRequest = body ของ POST /api/namespaces
// name จะถูกเอาไปตั้งเป็นชื่อ namespace จริงบน k8s เลย เลยต้องผ่านกฎ DNS-1123
// (ตัวเล็ก/ตัวเลข/ขีดกลาง, ขึ้นต้น-ลงท้ายด้วยตัวอักษรหรือตัวเลข) — เช็คด้วย regex ใน controller
// type เลือกได้ว่าจะใช้คนเดียว (solo) หรือรวมกลุ่ม (group)
// data flow: JSON จาก client → NamespaceController.Create → NamespaceManager.Create
type CreateNamespaceRequest struct {
	Name string `json:"name" binding:"required,min=3,max=50"`
	Type string `json:"type" binding:"required,oneof=solo group"`
}

// JoinNamespaceRequest = body ของ POST /api/namespaces/join — เข้าร่วม space แบบกลุ่มที่มีอยู่แล้ว
// data flow: JSON จาก client → NamespaceController.Join → NamespaceManager.Join (UPDATE users.namespace_id)
type JoinNamespaceRequest struct {
	NamespaceID int `json:"namespace_id" binding:"required,min=1"`
}

// SetQuotaRequest = body ของ PATCH /api/admin/namespaces/:id/quota (admin เท่านั้น)
// เพดานจริงถูกบังคับอีกชั้นใน NamespaceManager.SetQuota (กลุ่ม ≤ 8 core, เดี่ยว ≤ 3 core)
// data flow: JSON จาก client → AdminController.SetNamespaceQuota → NamespaceManager.SetQuota
type SetQuotaRequest struct {
	CPULimitMilli int `json:"cpu_limit_milli" binding:"required,min=100,max=8000"`
	RAMLimitMB    int `json:"ram_limit_mb" binding:"required,min=128,max=8192"`
	MaxServices   int `json:"max_services" binding:"required,min=1,max=10"`
}
