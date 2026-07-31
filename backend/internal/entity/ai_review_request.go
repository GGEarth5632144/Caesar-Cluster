package entity

import "time"

// AIReviewRequest = "ใบเสร็จ" ของ deploy request ที่ถูกส่งเข้า Cluster-AI เพื่อรอ review
//
// เหตุผลที่มี table นี้: Cluster-AI (คนละ repo, คนละระบบ) เก็บแค่ "สถานะ" ของ pipeline
// (queued/reviewing/needs_confirmation/...) ไว้ใน memory ล้วนๆ ไม่มี DB — ไม่ได้เก็บ
// service_name/image/cpu/ram ที่ Caesar-Cluster ส่งไปตอน submit ด้วยซ้ำ (ดู Cluster-AI/Backend/store/review_store.go)
// ฝั่ง frontend (AIReviewPage.tsx) พึ่ง React Router location.state ล้วนๆ เพื่อจำข้อมูลพวกนี้ระหว่างรอผล —
// ถ้าหน้าโดน refresh หรือเปิดลิงก์ตรง state จะหายหมด แล้วกด deploy จริงตอนท้ายไม่ได้เลยเพราะไม่รู้ว่าจะ deploy อะไร
//
// table นี้เลยมีไว้เป็นที่เดียวที่ยังหาข้อมูลตั้งต้นของ request กลับมาได้ — ไม่ใช่ system of record ของสถานะ
// pipeline (นั่นยังคง poll ตรงไปที่ Cluster-AI เหมือนเดิมทุกอย่าง ไม่มีอะไรเปลี่ยน) แค่จำ "ขอ deploy อะไรมา" เฉยๆ
//
// data flow เข้า: RequestQuotar.tsx (handleDeployWithAI) ยิง POST มาที่นี่ "คู่ขนาน" กับตอนที่ยิง POST /api/review
// ไปที่ Cluster-AI โดยตรง (คนละ call กัน ไม่ block กัน — Caesar-Cluster ไม่ได้ proxy การเรียก Cluster-AI)
// data flow ออก: AIReviewPage.tsx ถ้า location.state หาย → GET /api/ai-review-requests/:request_id แทน
//
// CPUMilli/RAMMB เป็นค่า snapshot ที่คำนวณไว้แล้วฝั่ง frontend ตอน submit (เหมือน entity.Service —
// ดูเหตุผลใน service.go) ไม่ใช่ค่าที่ resolve ใหม่จาก RequestTemplateID ตอนอ่าน เพราะ template อาจถูกลบ/ปิดไปแล้ว
type AIReviewRequest struct {
	ID                int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	RequestID         string    `gorm:"column:request_id;type:varchar(64);not null;uniqueIndex" json:"request_id"`
	NamespaceID       int       `gorm:"column:namespace_id;type:integer;not null;index:idx_ai_review_requests_ns" json:"namespace_id"`
	CreatedBy         int       `gorm:"column:created_by;type:integer;not null" json:"created_by"`
	ServiceName       string    `gorm:"column:service_name;type:varchar(50);not null" json:"service_name"`
	Image             string    `gorm:"column:image;type:varchar(200);not null" json:"image"`
	RequestTemplateID *int      `gorm:"column:request_template_id;type:integer" json:"request_template_id"`
	CPUMilli          int       `gorm:"column:cpu_milli;type:integer;not null;check:cpu_milli > 0" json:"cpu_milli"`
	RAMMB             int       `gorm:"column:ram_mb;type:integer;not null;check:ram_mb > 0" json:"ram_mb"`
	EnvVars           EnvVarMap `gorm:"column:env_vars;type:jsonb;not null;default:'{}'" json:"env_vars"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "ai_review_requests"
func (AIReviewRequest) TableName() string { return "ai_review_requests" }
