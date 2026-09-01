package dto

// MarkAlertsReadRequest = body ของ PATCH /api/alerts/read
//
// ส่ง ids มาเจาะจงเป็นรายตัว (ผู้ใช้กดอ่านทีละอัน) หรือ all=true เพื่อเคลียร์ทั้งหมดในคลิกเดียว
// (ปุ่ม "อ่านทั้งหมด" บนหัวหน้า Alerts) — ต้องมาอย่างใดอย่างหนึ่ง controller เช็คให้
type MarkAlertsReadRequest struct {
	IDs []int `json:"ids"`
	All bool  `json:"all"`
}
