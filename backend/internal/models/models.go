package models

import "time"

type User struct {
	ID        int       `json:"id"`
	StudentID string    `json:"student_id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type Node struct {
	ID         int    `json:"id"`
	Hostname   string `json:"hostname"`
	TotalCores int    `json:"total_cores"`
	TotalRAMMB int    `json:"total_ram_mb"`
	Status     string `json:"status"`
}

type VM struct {
	ID        int       `json:"id"`
	OwnerID   int       `json:"owner_id"`
	NodeID    int       `json:"node_id"`
	Name      string    `json:"name"`
	CPUCores  int       `json:"cpu_cores"`
	RAMMB     int       `json:"ram_mb"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateVMRequest struct {
	Name     string `json:"name" binding:"required,min=3,max=50"`
	CPUCores int    `json:"cpu_cores" binding:"required,min=1,max=8"`
	RAMMB    int    `json:"ram_mb" binding:"required,min=512,max=16384"`
}
