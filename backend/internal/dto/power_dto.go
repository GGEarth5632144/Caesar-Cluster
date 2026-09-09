package dto

import "time"

type KWSChild struct {
	ID       int    `json:"id"`
	ModbusID int    `json:"ModbusID"`
	Status   string `json:"Status"`
	Volt     string `json:"Volt"`
	Amp      string `json:"Amp"`
	Hz       string `json:"Hz"`
	PF       string `json:"PF"`
	Whr      string `json:"Whr"`
}

type KWSResponse struct {
	ID        int        `json:"id"`
	Text      string     `json:"Text"`
	Timestamp string     `json:"Timestamp"`
	Children  []KWSChild `json:"Children"`
}

type PowerHistoryResponse struct {
	Time      time.Time `json:"time"`      
	TotalWatt float64   `json:"totalWatt"`
	TotalAmp  float64   `json:"totalAmp"`
	AvgVolt   float64   `json:"avgVolt"`
}