package services

import (
	"context"
	"log"
	"time"

	"backend/internal/entity"
)

// MockProvisioner = provisioner ปลอมสำหรับ dev/test — ไม่แตะ cluster จริง แค่ log แล้วคืน success
// ใช้ตอน PROVISIONER=mock (ค่า default) เพื่อให้พัฒนา/เทสต์ API ได้โดยไม่ต้องมี k8s
type MockProvisioner struct{}

// NewMockProvisioner สร้าง mock — ถูกเลือกใช้ใน main เมื่อ PROVISIONER != "kubernetes"
func NewMockProvisioner() *MockProvisioner { return &MockProvisioner{} }

// EnsureNamespace จำลองการสร้าง namespace + ResourceQuota
// data flow: รับ namespace ที่ NamespaceManager เพิ่งบันทึกลง DB → log โควตาที่จะไปตั้งบน cluster → คืน nil
func (m *MockProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	log.Printf("[MOCK] สร้าง namespace '%s' quota: %dm CPU / %d MB",
		ns.Name, ns.CPULimitMilli, ns.RAMLimitMB)
	return nil
}

// DeleteNamespace จำลองการลบ namespace ทั้งก้อน
// data flow: รับชื่อ namespace จาก NamespaceManager → log → คืน nil
func (m *MockProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	log.Printf("[MOCK] ลบ namespace '%s'", nsName)
	return nil
}

// DeployService จำลองการ deploy workload: log สเปกที่ ServiceManager ส่งมา แจก node port ปลอมๆ แล้ว sleep ให้เหมือนมี latency จริง
// data flow: รับชื่อ namespace + service (สเปก snapshot แล้ว) จาก ServiceManager.Create → log
// → เซ็ต svc.NodePort (จำลองพฤติกรรมของ k8s Service ชนิด NodePort) → คืน nil
func (m *MockProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	log.Printf("[MOCK] deploy service '%s' (image=%s) เข้า namespace '%s' — %dm CPU / %d MB",
		svc.Name, svc.Image, nsName, svc.CPUMilli, svc.RAMMB)
	time.Sleep(300 * time.Millisecond) // จำลองว่าใช้เวลา

	port := 30000 + (svc.ID % 2768) // เลขปลอมแต่นิ่งต่อ service เดิม อยู่ในช่วง NodePort ของ k8s
	svc.NodePort = &port
	log.Printf("[MOCK] service '%s' เข้าถึงได้ที่ <node-ip>:%d", svc.Name, port)
	return nil
}

// DeleteService จำลองการลบ workload ตัวเดียว
// data flow: รับ namespace + ชื่อ service จาก ServiceManager.Delete → log → คืน nil
func (m *MockProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	log.Printf("[MOCK] ลบ service '%s' ออกจาก namespace '%s'", svcName, nsName)
	return nil
}
