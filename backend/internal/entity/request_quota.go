package entity

import "time"

type request_quota struct {
	RequestID        string    `json:"request_id"`
	NamespaceID      int       `json:"namespace_id"`
	CPUlimit          int       `json:"cpu_limit"`
	Ramlimit          int       `json:"ram_limit"`
	CreatedAt         time.Time `json:"created_at"`
}