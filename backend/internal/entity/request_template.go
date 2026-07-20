package entity

import "time"

// RequestTemplate = ตาราง request_templates — choice สำเร็จรูปให้ user เลือก...
// (คอมเมนต์ส่วนบนเขียนไว้ดีแล้ว นำมาใส่เหมือนเดิมได้เลยครับ)
type RequestTemplate struct {
	ID              int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	OptionName      string    `gorm:"column:name;type:varchar(50);unique;not null" json:"option_name"`
	Category        string    `gorm:"column:category;type:varchar(50);not null" json:"category"`
	Description     string    `gorm:"column:description;type:varchar(255);not null" json:"description"`
	RelateSubject   string    `gorm:"column:relate_subject;type:varchar(50);not null" json:"relate_subject"`
	CPULimitMilli   int       `gorm:"column:cpu_limit_milli;type:integer;not null;check:cpu_limit_milli > 0" json:"cpu_limit_milli"`
	RAMLimitMB      int       `gorm:"column:ram_limit_mb;type:integer;not null;check:ram_limit_mb > 0" json:"ram_limit_mb"`
	StorageGB       int       `gorm:"column:storage_gb;type:integer;not null;check:storage_gb > 0" json:"storage_gb"` // ระบุหน่วยให้ชัดเจน
	IsActive        bool      `gorm:"column:is_active;type:boolean;not null;default:true" json:"is_active"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "request_templates"
func (RequestTemplate) TableName() string { return "request_templates" }