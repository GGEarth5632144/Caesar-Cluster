package entity

import (
	"time"
)

type NodeTelemetry struct {
	ID          uint      `gorm:"primaryKey"`
	NodeName    string    `gorm:"uniqueIndex;not null"` // ตั้งเป็น Unique เพื่อใช้อัปเดตข้อมูลโหนดเดิม
	Temperature float64   `gorm:"not null"`
	RamUsedMB   float64   `gorm:"not null"`
	RamTotalMB  float64   `gorm:"not null;default:0"` 
    CpuUsage    float64   `gorm:"not null;default:0"`
	UseableStorage     float64   `gorm:"not null;default:0"`
	IsUp        int       `gorm:"not null"`
	Procs       int       `gorm:"not null"`
	UpdatedAt   time.Time
}