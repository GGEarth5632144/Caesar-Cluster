package entity

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONB คือ custom type สำหรับ map เข้ากับ column ประเภท jsonb ของ Postgres
// เขียนเองด้วย stdlib ล้วน (database/sql/driver + encoding/json) ไม่ต้องพึ่ง
// gorm.io/datatypes เพิ่ม เพราะ field นี้ยังไม่ถูกใช้งานจริงในโค้ด แค่รักษา column ไว้ให้ตรงกับ DB เดิม
type JSONB map[string]any

// Value แปลง map ในหน่วยความจำ → bytes ก่อนเขียนลง DB (ฝั่ง "ส่งออก")
// data flow: GORM เรียกตอน INSERT/UPDATE → json.Marshal(map) → driver.Value ([]byte) ลง column jsonb
// map ว่าง/nil คืน nil เพื่อให้เก็บเป็น NULL ไม่ใช่สตริง "null"
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan แปลง bytes จาก DB → map ในหน่วยความจำ (ฝั่ง "รับเข้า")
// data flow: GORM เรียกตอน SELECT → รับ []byte จาก column jsonb → json.Unmarshal ใส่กลับเป็น map
// ค่า NULL (nil) แปลงเป็น map nil, ชนิดอื่นที่ไม่ใช่ []byte ถือว่า error
func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("entity: JSONB.Scan ต้องการ []byte")
	}
	return json.Unmarshal(b, j)
}
