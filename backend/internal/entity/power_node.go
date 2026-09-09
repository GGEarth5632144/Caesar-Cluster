package entity

import "time"

type PowerNode struct {
	ModbusID  int       `gorm:"primaryKey"`
	Status    string    `gorm:"not null"`
	Volt      float64   `gorm:"not null"`
	Amp       float64   `gorm:"not null"`
	Hz        float64   `gorm:"not null"`
	PF        float64   `gorm:"not null"`
	Whr       float64   `gorm:"not null"`
	Watt      float64   `gorm:"not null"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type PowerHistory struct {
	ID        uint      `gorm:"primaryKey"`
	Timestamp time.Time `gorm:"index"`
	TotalWatt float64	`gorm:"not null"`
	TotalAmp  float64	`gorm:"not null"`
	AvgVolt   float64	`gorm:"not null"`
}