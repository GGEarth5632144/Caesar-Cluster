package dto

// CreatePlanRequest = body ของ POST /api/admin/plans — admin สร้าง "choice" ให้ผู้ใช้เลือก
// เพดานตรงกับเพดานของ service 1 ตัว (3000m / 2048MB) เพราะ plan ก็คือสเปกของ service ตัวหนึ่ง
// data flow: JSON จาก client → AdminController.CreatePlan → INSERT plans
type CreatePlanRequest struct {
	Name     string `json:"name" binding:"required,min=2,max=50"`
	CPUMilli int    `json:"cpu_milli" binding:"required,min=100,max=3000"`
	RAMMB    int    `json:"ram_mb" binding:"required,min=128,max=2048"`
}
