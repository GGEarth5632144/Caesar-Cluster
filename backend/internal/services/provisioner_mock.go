package services

import (
	"context"
	"log"
	"time"
)

type MockProvisioner struct{}

func NewMockProvisioner() *MockProvisioner { return &MockProvisioner{} }

func (m *MockProvisioner) CreateVM(ctx context.Context, nodeHostname, vmName string, cores, ramMB int) error {
	log.Printf("[MOCK] สร้าง VM '%s' บน %s (%d cores, %d MB)", vmName, nodeHostname, cores, ramMB)
	time.Sleep(500 * time.Millisecond) // จำลองว่าใช้เวลา
	return nil
}

func (m *MockProvisioner) DeleteVM(ctx context.Context, nodeHostname string, vmID int) error {
	log.Printf("[MOCK] ลบ VM id=%d บน %s", vmID, nodeHostname)
	return nil
}
