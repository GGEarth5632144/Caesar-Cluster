package entity

import "time"

// Plan = ตาราง plans — คือ "choices" ที่ admin สร้างไว้ให้ผู้ใช้เลือก (แบบเดียวกับแพ็กเกจของ PebbleHost
// แต่ไม่มีค่าใช้จ่าย) เช่น small = 500m/512MB, max = 3000m/2048MB
//
// ข้อมูลไหลเข้า: admin สร้างผ่าน POST /api/admin/plans (cmd/seed ใส่ชุดตั้งต้นให้ก่อน)
// ข้อมูลไหลออก: user ดึงรายการที่ is_active ผ่าน GET /api/plans ไปเลือกตอน deploy
// → ServiceManager.Create ก๊อป cpu_milli/ram_mb ของ plan มาเก็บลง service (snapshot)
//
// ทำไมต้อง snapshot: ถ้า service อ้าง plan_id อย่างเดียว แล้ววันหลัง admin แก้สเปกของ plan
// โควตาที่ service กินอยู่จะเปลี่ยนย้อนหลังทันทีโดยไม่มีใครสั่ง — เลยเก็บค่าจริงไว้ที่ service ด้วย
type Plan struct {
	ID        int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Name      string    `gorm:"column:name;type:varchar(50);unique;not null" json:"name"`
	CPUMilli  int       `gorm:"column:cpu_milli;type:integer;not null;check:cpu_milli > 0" json:"cpu_milli"`
	RAMMB     int       `gorm:"column:ram_mb;type:integer;not null;check:ram_mb > 0" json:"ram_mb"`
	IsActive  bool      `gorm:"column:is_active;type:boolean;not null;default:true" json:"is_active"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "plans"
func (Plan) TableName() string { return "plans" }
