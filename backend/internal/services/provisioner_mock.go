package services

import (
	"context"
	"fmt"
	"io"
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
// คืน nil เสมอ = idempotent ตามสัญญาใน Provisioner อยู่แล้ว (สั่งลบซ้ำกี่รอบก็ผ่าน)
func (m *MockProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	log.Printf("[MOCK] ลบ namespace '%s'", nsName)
	return nil
}

// DeployService จำลองการ deploy workload: log สเปกที่ ServiceManager ส่งมา แจก node port ปลอมๆ แล้ว sleep ให้เหมือนมี latency จริง
// data flow: รับชื่อ namespace + service (สเปก snapshot แล้ว) จาก ServiceManager.Create → log
// → เซ็ต svc.NodePort (จำลองพฤติกรรมของ k8s Service ชนิด NodePort) → คืน nil
func (m *MockProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	log.Printf("[MOCK] deploy service '%s' (image=%s) เข้า namespace '%s' — %d replica × (%dm CPU / %d MB) / port %d / %d env vars",
		svc.Name, svc.Image, nsName, svc.Replicas, svc.CPUMilli, svc.RAMMB, svc.ContainerPort, len(svc.EnvVars))
	time.Sleep(300 * time.Millisecond) // จำลองว่าใช้เวลา

	port := 30000 + (svc.ID % 2768) // เลขปลอมแต่นิ่งต่อ service เดิม อยู่ในช่วง NodePort ของ k8s
	svc.NodePort = &port
	log.Printf("[MOCK] service '%s' เข้าถึงได้ที่ <node-ip>:%d → container port %d",
		svc.Name, port, svc.ContainerPort)
	return nil
}

// ScaleService จำลองการปรับจำนวน Pod — ของจริงคือแก้ Deployment.spec.replicas เฉยๆ
func (m *MockProvisioner) ScaleService(ctx context.Context, nsName, svcName string, replicas int) error {
	log.Printf("[MOCK] scale service '%s' ใน namespace '%s' เป็น %d replica", svcName, nsName, replicas)
	return nil
}

// DeleteService จำลองการลบ workload ตัวเดียว
// data flow: รับ namespace + ชื่อ service จาก ServiceManager.Delete → log → คืน nil
func (m *MockProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	log.Printf("[MOCK] ลบ service '%s' ออกจาก namespace '%s'", svcName, nsName)
	return nil
}

// Logs จำลอง log ของ container ด้วยข้อความปลอมที่รูปแบบเหมือนจริง (มี timestamp นำหน้าแบบเดียวกับ
// ที่ KubernetesProvisioner ส่งตอน Timestamps=true) เพื่อให้พัฒนา/ทดสอบหน้า log viewer ได้
// โดยไม่ต้องมีคลัสเตอร์จริง
//
// ใช้ io.Pipe เขียนจาก goroutine แยก: ส่งบรรทัดเริ่มต้นก่อน แล้วถ้า opts.Follow=true จะไม่ปิด
// stream แต่ส่งบรรทัดใหม่ทุก 1.5 วินาทีจนกว่า ctx จะถูก cancel — เลียนพฤติกรรม "ไหลสด" ของจริง
func (m *MockProvisioner) Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	tail := opts.TailLines
	if tail <= 0 {
		tail = 20
	}

	go func() {
		defer pw.Close()

		// เขียนบรรทัดหนึ่ง คืน false ถ้าฝั่งอ่านปิดไปแล้ว (ผู้เรียก Close() ตัว io.ReadCloser
		// ที่คืนไป) — ต้องเช็คทุกครั้งไม่งั้น goroutine นี้จะเขียนเข้า pipe ที่ตายแล้วค้างไปตลอดกาล
		writeLine := func(msg string) bool {
			line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339Nano), msg)
			_, err := pw.Write([]byte(line))
			return err == nil
		}

		if !writeLine(fmt.Sprintf("[mock] starting container for service %q in namespace %q", svcName, nsName)) {
			return
		}
		if !writeLine("[mock] listening on 0.0.0.0:8080") {
			return
		}
		for i := int64(0); i < tail; i++ {
			if !writeLine(mockLogLine(i)) {
				return
			}
		}
		if !opts.Follow {
			return
		}

		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for n := tail; ; n++ {
			select {
			case <-ctx.Done(): // ผู้ใช้ปิดหน้าเว็บ/เปลี่ยนหน้า — HTTP request context ถูก cancel
				return
			case <-ticker.C:
				if !writeLine(mockLogLine(n)) {
					return
				}
			}
		}
	}()

	return pr, nil
}

// mockLogLine สร้างเนื้อ log ปลอมของบรรทัดที่ n — ส่วนใหญ่เป็น access log ปกติ
// แทรก error/warning เป็นระยะ เพราะ LogAlertScanner อ่าน log จาก provisioner ตัวเดียวกันนี้
// ถ้า mock พ่นแต่ "GET / 200" ล้วน ฟีเจอร์แจ้งเตือนจะทดสอบบนเครื่อง dev ไม่ได้เลย
//
// ใช้ n % k แทนการสุ่ม เพื่อให้ผลลัพธ์นิ่งพอที่เทสต์จะยืนยันจำนวนได้
func mockLogLine(n int64) string {
	switch {
	case n%17 == 16:
		return "[mock] level=error msg=\"upstream connection refused\" upstream=127.0.0.1:5432 attempt=" +
			fmt.Sprint(n)
	case n%11 == 10:
		return fmt.Sprintf("[mock] 10.244.0.1 - - \"GET /api/items HTTP/1.1\" 503 0 %dms", 800+n%200)
	case n%7 == 6:
		return fmt.Sprintf("[mock] level=warn msg=\"request took longer than expected\" duration=%dms", 900+n%100)
	default:
		return fmt.Sprintf("[mock] GET / 200 %dms", 5+n%30)
	}
}
