package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"backend/internal/entity"
)

// MockProvisioner = provisioner ปลอมสำหรับ dev/test — ไม่แตะ cluster จริง แค่ log แล้วคืน success
// ใช้ตอน PROVISIONER=mock (ค่า default) เพื่อให้พัฒนา/เทสต์ API ได้โดยไม่ต้องมี k8s
//
// Status ตอบเฉพาะสิ่งที่ตัวเองเห็นมากับตา ไม่แกล้งทำเป็น crash loop หรือ pending ให้ เพราะ mock
// ไม่มีทางรู้ว่า container จะขึ้นได้ไหม — ตัวที่ตรวจจริงคือ KubernetesProvisioner.Status
type MockProvisioner struct {
	mu sync.Mutex
	// key = nsName + "/" + svcName; true = deploy อยู่, false = ถูกลบไปแล้ว
	// "ไม่มี key" ต่างจาก false: แปลว่า mock ไม่เคยเห็น workload นี้เลย จึงตอบแทนคลัสเตอร์ไม่ได้
	known map[string]bool
	// credential ของ database template แทน k8s Secret — อยู่ในหน่วยความจำ รีสตาร์ทแล้วหาย (รับได้สำหรับ dev)
	creds map[string]DatabaseCredentials
}

// NewMockProvisioner สร้าง mock — ถูกเลือกใช้ใน main เมื่อ PROVISIONER != "kubernetes"
func NewMockProvisioner() *MockProvisioner {
	return &MockProvisioner{known: make(map[string]bool), creds: make(map[string]DatabaseCredentials)}
}

func mockKey(nsName, svcName string) string { return nsName + "/" + svcName }

// EnsureNamespace จำลองการสร้าง namespace + ResourceQuota
// data flow: รับ namespace ที่ NamespaceManager เพิ่งบันทึกลง DB → log โควตาที่จะไปตั้งบน cluster → คืน nil
func (m *MockProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	log.Printf("[MOCK] สร้าง namespace '%s' quota: %dm CPU / %d MB RAM / %d MB ดิสก์",
		ns.Name, ns.CPULimitMilli, ns.RAMLimitMB, ns.StorageLimitMB)
	return nil
}

// DeleteNamespace จำลองการลบ namespace ทั้งก้อน
// data flow: รับชื่อ namespace จาก NamespaceManager → log → คืน nil
// คืน nil เสมอ = idempotent ตามสัญญาใน Provisioner อยู่แล้ว (สั่งลบซ้ำกี่รอบก็ผ่าน)
func (m *MockProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	log.Printf("[MOCK] ลบ namespace '%s'", nsName)
	return nil
}

// DeployService จำลองการ deploy: log สเปกแล้ว sleep ให้เหมือนมี latency จริง
//
// แยกทางเดินตามสองแกนให้ตรงกับของจริง (ดูสัญญาใน Provisioner) — ข้อที่หน้าเว็บต้องรับมือคือ
// database ไม่ได้ NodePort (null ตลอดชีวิต) ส่วน web ที่มีดิสก์ยังได้ NodePort ตามปกติ
// ทำให้ทดสอบได้ตั้งแต่รัน mock ว่าหน้าเว็บแยกสองเคสนี้ถูกต้องไหม
func (m *MockProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	// จดไว้ว่าเคย deploy แล้ว เพื่อให้ Status ตอบได้ว่าของยังอยู่ไหม
	m.mu.Lock()
	m.known[mockKey(nsName, svc.Name)] = true
	if svc.IsTemplateDatabase() {
		m.creds[mockKey(nsName, svc.Name)] = DatabaseCredentials{Username: svc.DBUsername, Password: svc.DBPassword, Database: svc.DBName}
	}
	m.mu.Unlock()

	// ของจริง: มีดิสก์ = StatefulSet + PVC (1 pod), ไม่มี = Deployment
	if svc.HasStorage() {
		log.Printf("[MOCK] deploy service '%s' (image=%s) เป็น StatefulSet เข้า namespace '%s' — %dm CPU / %d MB / ดิสก์ %d MB ที่ %s / port %d / %d env vars",
			svc.Name, svc.Image, nsName, svc.CPUMilli, svc.RAMMB, svc.StorageMB, svc.DataPath, svc.ContainerPort, len(svc.EnvVars))
	} else {
		log.Printf("[MOCK] deploy service '%s' (image=%s) เข้า namespace '%s' — %d replica × (%dm CPU / %d MB) / port %d / %d env vars",
			svc.Name, svc.Image, nsName, svc.Replicas, svc.CPUMilli, svc.RAMMB, svc.ContainerPort, len(svc.EnvVars))
	}
	time.Sleep(300 * time.Millisecond) // จำลองว่าใช้เวลา

	// database "ไม่" ได้ NodePort ตามสัญญาใน Provisioner — ของจริงเปิดแค่ ClusterIP + NetworkPolicy
	if svc.IsDatabase {
		log.Printf("[MOCK] database '%s' เข้าถึงได้เฉพาะใน namespace ที่ %s:%d (ClusterIP เท่านั้น)",
			svc.Name, svc.Name, svc.ContainerPort)
		return nil
	}

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

// UpdateService จำลองการแก้ workload — กติกาเดียวกับของจริง: มีดิสก์ห้ามแตะค่าที่ผูกกับ PVC,
// ชื่อ/ชนิดเท่าเดิม = แก้ในที่ (NodePort เลขเดิม), ไม่มีดิสก์แล้วเปลี่ยนชื่อ/ขอดิสก์ = ลบแล้วสร้างใหม่
func (m *MockProvisioner) UpdateService(ctx context.Context, nsName string, oldSvc, svc *entity.Service) error {
	if oldSvc.HasStorage() {
		if msg := storageChange(oldSvc, svc); msg != "" {
			return fmt.Errorf("%w: %s", ErrStorageImmutable, msg)
		}
	} else if svc.Name != oldSvc.Name || svc.HasStorage() {
		if err := m.DeleteService(ctx, nsName, oldSvc); err != nil {
			return err
		}
		return m.DeployService(ctx, nsName, svc)
	}

	m.mu.Lock()
	m.known[mockKey(nsName, svc.Name)] = true
	m.mu.Unlock()

	log.Printf("[MOCK] แก้ service '%s' ใน namespace '%s' ในที่ (image=%s, %dm CPU / %d MB, port %d) — PVC และ NodePort คงเดิม",
		svc.Name, nsName, svc.Image, svc.CPUMilli, svc.RAMMB, svc.ContainerPort)
	if !svc.IsDatabase {
		port := 30000 + (svc.ID % 2768) // เลขเดียวกับที่ DeployService จ่าย = "คงเดิม"
		svc.NodePort = &port
	}
	return nil
}

// ErrMockNoRecord = mock ถูกถามถึง workload ที่ตัวเองไม่เคยเห็น จึงตอบแทนคลัสเตอร์ไม่ได้
//
// ต้องเป็น error ไม่ใช่ PhaseGone: mock เก็บทุกอย่างไว้ในหน่วยความจำ พอรีสตาร์ท backend
// มันจำ service ที่ deploy ไว้ก่อนหน้าไม่ได้เลย ถ้าตอบ PhaseGone (= ยืนยันแล้วว่าของหาย)
// monitor จะไล่เขียน failed ทับ service ทั้งระบบในรอบเดียวทั้งที่ไม่มีอะไรพัง
// และ failed เป็นสถานะนิ่ง จึงไม่มีวันกลับมาเอง
var ErrMockNoRecord = errors.New("mock ไม่มีบันทึกของ workload นี้ (เพิ่งรีสตาร์ท?) จึงบอกสถานะแทนคลัสเตอร์ไม่ได้")

// Status ตอบเท่าที่ mock รู้จริง: deploy เอง = running, ลบเอง = gone, ไม่เคยเห็น = ตอบไม่ได้
// บน PROVISIONER=mock จึงไม่มีทางเห็นสถานะ crashloop/pending เลย ซึ่งถูกแล้ว เพราะไม่มีคลัสเตอร์จริง
func (m *MockProvisioner) Status(_ context.Context, nsName string, svc *entity.Service) (WorkloadStatus, error) {
	m.mu.Lock()
	deployed, recorded := m.known[mockKey(nsName, svc.Name)]
	m.mu.Unlock()

	switch {
	case !recorded:
		return WorkloadStatus{}, ErrMockNoRecord
	case !deployed:
		return WorkloadStatus{
			Phase:   PhaseGone,
			Reason:  "NotFound",
			Message: "workload นี้ถูกลบไปแล้ว",
		}, nil
	default:
		return WorkloadStatus{Phase: PhaseRunning}, nil
	}
}

// DeleteService จำลองการลบ workload ตัวเดียว — มีดิสก์ต้องถอน PVC เพิ่ม, database ถอน NetworkPolicy เพิ่ม
func (m *MockProvisioner) DeleteService(ctx context.Context, nsName string, svc *entity.Service) error {
	m.mu.Lock()
	m.known[mockKey(nsName, svc.Name)] = false
	delete(m.creds, mockKey(nsName, svc.Name))
	m.mu.Unlock()

	switch {
	case svc.IsDatabase:
		log.Printf("[MOCK] ลบ database '%s' ออกจาก namespace '%s' — StatefulSet + NetworkPolicy '%s' + PVC '%s' (%d MB คืนโควตาดิสก์)",
			svc.Name, nsName, networkPolicyName(svc.Name), pvcName(svc.Name), svc.StorageMB)
	case svc.HasStorage():
		log.Printf("[MOCK] ลบ service '%s' ออกจาก namespace '%s' — StatefulSet + PVC '%s' (%d MB คืนโควตาดิสก์)",
			svc.Name, nsName, pvcName(svc.Name), svc.StorageMB)
	default:
		log.Printf("[MOCK] ลบ service '%s' ออกจาก namespace '%s'", svc.Name, nsName)
	}
	return nil
}

// DatabaseCredentials คืน credential ที่จำไว้ตอน DeployService (แทนการอ่าน Secret)
// ไม่เคยเห็น (เช่น backend รีสตาร์ท) = ErrMockNoRecord ด้วยเหตุผลเดียวกับ Status
func (m *MockProvisioner) DatabaseCredentials(_ context.Context, nsName string, svc *entity.Service) (DatabaseCredentials, error) {
	if !svc.IsTemplateDatabase() {
		return DatabaseCredentials{}, ErrNotTemplateDatabase
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cred, ok := m.creds[mockKey(nsName, svc.Name)]
	if !ok {
		return DatabaseCredentials{}, ErrMockNoRecord
	}
	return cred, nil
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
// แทรก error/warning เป็นระยะ ให้หน้า log viewer มีของจริงให้ดูตอน dev
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
