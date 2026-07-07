package services

import "context"

// Provisioner คือสัญญาว่า "ตัวสร้าง VM จริง" ต้องทำอะไรได้บ้าง
type Provisioner interface {
	CreateVM(ctx context.Context, nodeHostname, vmName string, cores, ramMB int) error
	DeleteVM(ctx context.Context, nodeHostname string, vmID int) error
}
