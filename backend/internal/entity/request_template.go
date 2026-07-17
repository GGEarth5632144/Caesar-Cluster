package entity

import "time"

// RequestTemplate = ตาราง request_templates — choice สำเร็จรูปให้ user เลือก ทั้งตอนยื่น Request (ขอ namespace/โควตา)
// และตอน deploy Service (เดิมสองอันนี้แยกเป็น Plan กับ RequestTemplate คนละตาราง แต่จริงๆ คือ concept เดียวกัน
// ทั้งคู่: "ทางเลือกสเปก CPU/RAM สำเร็จรูปที่ admin เปิดไว้ให้เลือก" เลยรวมมาเหลือตารางเดียวตาม ERD)
//
// ตัดฟิลด์ contributor_id ออกจาก ERD ตั้งต้น — template เป็น choice กลาง ใครก็เลือกได้หลายคน
// ไม่ควรผูกกับ user คนใดคนหนึ่งไว้ล่วงหน้า (ผิดกับ Request ที่ผูกกับ user_id ของคนยื่นจริง)
//
// is_active ไม่ได้อยู่ใน ERD ตั้งต้น แต่จำเป็นต้องมี — สืบทอดมาจาก Plan เดิม เพื่อให้ admin "ปิด" choice ได้
// โดยไม่ต้องลบแถวทิ้ง (ลบไม่ได้อยู่แล้วถ้ามี Service/Request เก่าอ้าง template_id นี้อยู่)
//
// ข้อมูลไหลเข้า: admin สร้างผ่าน POST /api/admin/request-templates
// ข้อมูลไหลออก: user ดึงรายการที่ is_active ไปเลือกได้ 2 จุด — ตอนยื่น Request (RequestController ก๊อป
// cpu_limit_milli/ram_limit_mb ลง Request) และตอน deploy Service (ServiceManager ก๊อปค่าเดียวกันลง Service)
// เป็น snapshot ทั้งคู่ — ดูเหตุผลเรื่อง snapshot ใน service.go
type RequestTemplate struct {
	ID            int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Name          string    `gorm:"column:name;type:varchar(50);unique;not null" json:"name"`
	CPULimitMilli int       `gorm:"column:cpu_limit_milli;type:integer;not null;check:cpu_limit_milli > 0" json:"cpu_limit_milli"`
	RAMLimitMB    int       `gorm:"column:ram_limit_mb;type:integer;not null;check:ram_limit_mb > 0" json:"ram_limit_mb"`
	IsActive      bool      `gorm:"column:is_active;type:boolean;not null;default:true" json:"is_active"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "request_templates"
func (RequestTemplate) TableName() string { return "request_templates" }
