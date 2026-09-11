package services

import (
	"context"
	"errors"
	"testing"

	"backend/internal/entity"
)

// TestTranslateWorkloadStatus — ตารางแปลงสภาพจากคลัสเตอร์เป็นสถานะที่ระบบเก็บ
//
// กติกาที่สำคัญที่สุดสองข้อ:
//  1. Running ต้องล้าง reason/message เก่าทิ้ง ไม่งั้นหน้าเว็บจะโชว์ข้อความ crash loop
//     ค้างอยู่บน service ที่กลับมารันปกติแล้ว
//  2. ทุกสถานะที่ไม่ใช่ Running ต้องมี reason เสมอ — ปล่อยว่างเท่ากับบอกผู้ใช้แค่ว่า "พัง"
func TestTranslateWorkloadStatus(t *testing.T) {
	cases := []struct {
		name       string
		in         WorkloadStatus
		wantStatus string
		wantReason string
	}{
		{
			name:       "รันอยู่ปกติ",
			in:         WorkloadStatus{Phase: PhaseRunning},
			wantStatus: entity.ServiceRunning,
			wantReason: "",
		},
		{
			name:       "ตายซ้ำๆ",
			in:         WorkloadStatus{Phase: PhaseCrashLoop, Reason: "CrashLoopBackOff", Restarts: 5},
			wantStatus: entity.ServiceCrashLoop,
			wantReason: "CrashLoopBackOff",
		},
		{
			name:       "ตายซ้ำๆ แต่คลัสเตอร์ไม่บอกสาเหตุ",
			in:         WorkloadStatus{Phase: PhaseCrashLoop},
			wantStatus: entity.ServiceCrashLoop,
			wantReason: "CrashLoopBackOff", // ต้องเติม default ให้ ไม่ปล่อยว่าง
		},
		{
			name:       "ไม่มี node ว่างให้ลง",
			in:         WorkloadStatus{Phase: PhasePending, Reason: "FailedScheduling"},
			wantStatus: entity.ServicePending,
			wantReason: "FailedScheduling",
		},
		{
			name:       "หายไปจากคลัสเตอร์",
			in:         WorkloadStatus{Phase: PhaseGone},
			wantStatus: entity.ServiceFailed,
			wantReason: "NotFound",
		},
		{
			name:       "พังแบบอื่น",
			in:         WorkloadStatus{Phase: PhaseFailed, Reason: "ImagePullBackOff"},
			wantStatus: entity.ServiceFailed,
			wantReason: "ImagePullBackOff",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason, message := translate(tc.in)
			if status != tc.wantStatus {
				t.Errorf("status = %q ต้องเป็น %q", status, tc.wantStatus)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q ต้องเป็น %q", reason, tc.wantReason)
			}
			if tc.wantStatus == entity.ServiceRunning && (reason != "" || message != "") {
				t.Errorf("กลับมารันปกติแล้วต้องล้างข้อความเก่าทิ้ง ได้ reason=%q message=%q",
					reason, message)
			}
			if tc.wantStatus != entity.ServiceRunning && reason == "" {
				t.Error("สถานะที่ไม่ใช่ running ต้องมีเหตุผลเสมอ")
			}
		})
	}
}

// TestServiceStatusSettled — หน้าเว็บใช้ตัวนี้ตัดสินว่ายังต้องดึงข้อมูลซ้ำอยู่ไหม
// crashloop ต้องนับว่า "ยังไม่นิ่ง" เพราะ container อาจกลับมาขึ้นได้เองหลัง backoff
func TestServiceStatusSettled(t *testing.T) {
	settled := []string{entity.ServiceRunning, entity.ServiceFailed}
	for _, s := range settled {
		if !entity.ServiceStatusSettled(s) {
			t.Errorf("%q ต้องถือว่านิ่งแล้ว", s)
		}
	}
	unsettled := []string{entity.ServiceCreating, entity.ServicePending, entity.ServiceCrashLoop}
	for _, s := range unsettled {
		if entity.ServiceStatusSettled(s) {
			t.Errorf("%q ต้องถือว่ายังไม่นิ่ง", s)
		}
	}
}

// stubProv = provisioner ปลอมสำหรับเทสต์ ที่บังคับให้ Status ตอบอะไรก็ได้ตามต้องการ
//
// อยู่ในไฟล์เทสต์โดยตั้งใจ ไม่ใช่ใน MockProvisioner ที่ shipped ไปกับระบบ:
// mock ต้องตอบแค่ที่รู้จริง ส่วนการแปลสภาพ pod ของจริงอยู่ที่ KubernetesProvisioner.Status
// (มีเทสต์ของตัวเองใน provisioner_k8s_test.go) — ที่นี่ทดสอบเฉพาะฝั่ง monitor
//
// ฝัง *MockProvisioner ไว้เพื่อได้เมธอดที่เหลือของ Provisioner ครบโดยไม่ต้องเขียนซ้ำ
//
// ns = ขอบเขตที่ยอมตอบ: checkOnce สแกน service "ทุกแถวใน DB" ไม่ได้จำกัด namespace (ถูกแล้ว
// สำหรับของจริง) แต่ DB ทดสอบเป็นตัวเดียวกับที่ dev รันเซิร์ฟเวอร์อยู่ ถ้า stub ตอบให้ทุกแถว
// เทสต์จะเขียนสถานะทับ service ของจริงที่ไม่เกี่ยวกัน — เคยทำให้เทสต์ล้มแบบสุ่มมาแล้ว
// ตอบ ErrMockNoRecord ให้แถวนอกขอบเขต = monitor คงสถานะเดิมไว้ ไม่แตะของใคร
type stubProv struct {
	*MockProvisioner
	ns     string
	status WorkloadStatus
}

func (p *stubProv) Status(_ context.Context, nsName string, _ *entity.Service) (WorkloadStatus, error) {
	if p.ns != "" && nsName != p.ns {
		return WorkloadStatus{}, ErrMockNoRecord
	}
	return p.status, nil
}

// TestMockStatusReportsOnlyWhatItKnows — mock ต้องไม่เดาแทนคลัสเตอร์
//
// ตอบได้แค่สิ่งที่ตัวเองเห็นมากับตา: deploy เอง = running, ลบเอง = gone
// ส่วนของที่ไม่เคยเห็นต้องตอบว่า "ไม่รู้" (error) ไม่ใช่ "ไม่มี" — ดู ErrMockNoRecord
// ถ้าวันไหนมีคนเติมการจำลอง pending หรือ crash loop กลับเข้ามา เทสต์นี้จะจับได้
func TestMockStatusReportsOnlyWhatItKnows(t *testing.T) {
	m := NewMockProvisioner()
	ctx := context.Background()
	svc := &entity.Service{
		ID: 1, Name: "web", Image: "nginx:latest",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 8080, Replicas: 1,
	}

	// ไม่เคยเห็น workload นี้ = ตอบแทนคลัสเตอร์ไม่ได้ ต้องไม่ใช่ PhaseGone
	if _, err := m.Status(ctx, "ns-a", svc); !errors.Is(err, ErrMockNoRecord) {
		t.Errorf("ของที่ mock ไม่เคยเห็นต้องได้ ErrMockNoRecord ได้ %v", err)
	}

	if err := m.DeployService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("deploy ไม่สำเร็จ: %v", err)
	}
	got, err := m.Status(ctx, "ns-a", svc)
	if err != nil {
		t.Fatalf("ของที่ deploy เองต้องตอบได้: %v", err)
	}
	if got.Phase != PhaseRunning {
		t.Errorf("deploy แล้วต้องได้ %q ได้ %q — mock ไม่ควรเดาสภาพอื่นแทนคลัสเตอร์",
			PhaseRunning, got.Phase)
	}

	// ลบเองแล้วต้องเป็น gone — อันนี้ mock รู้จริง กันไม่ให้รายงานของที่ถูกลบว่ายังอยู่
	if err := m.DeleteService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("ลบไม่สำเร็จ: %v", err)
	}
	got, err = m.Status(ctx, "ns-a", svc)
	if err != nil {
		t.Fatalf("ของที่ลบเองต้องตอบได้: %v", err)
	}
	if got.Phase != PhaseGone {
		t.Errorf("ลบแล้วต้องได้ %q ได้ %q", PhaseGone, got.Phase)
	}
}

// TestRestartDoesNotFailEveryService — regression test ของบั๊กที่เจอตอนตรวจระบบ
//
// อาการ: รีสตาร์ท backend บน PROVISIONER=mock แล้ว service ทุกตัวในระบบกลายเป็น failed
// ภายในรอบเช็คเดียว พร้อมข้อความว่าไม่พบ workload บนคลัสเตอร์ ทั้งที่ไม่มีอะไรพังเลย —
// และ failed เป็นสถานะนิ่ง จึงค้างแบบนั้นถาวรจนกว่าจะลบทิ้งสร้างใหม่
//
// ต้นเหตุ: mock เก็บรายการที่ deploy ไว้ในหน่วยความจำ process ใหม่จึงจำอะไรไม่ได้เลย
// แล้วดันรายงานความจำเสื่อมของตัวเองเป็น PhaseGone ซึ่งแปลว่า "ยืนยันแล้วว่าของหาย"
func TestRestartDoesNotFailEveryService(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "health-restart-test-ns")
	ctx := context.Background()

	prov := NewMockProvisioner()
	mgr := NewServiceManager(db, NewQuotaService(db), prov)

	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "survivor", Image: "nginx:latest", CPUMilli: 300, RAMMB: 256,
	})
	if err != nil {
		t.Fatalf("สร้าง service ไม่สำเร็จ: %v", err)
	}

	NewServiceHealthMonitor(db, prov).checkOnce(ctx)
	var after entity.Service
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service ไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceRunning {
		t.Fatalf("ก่อนรีสตาร์ทต้องเป็น running ได้ %q", after.Status)
	}

	// รีสตาร์ท = provisioner ตัวใหม่ที่ความจำว่างเปล่า ส่วนแถวใน DB ยังอยู่ครบ
	NewServiceHealthMonitor(db, NewMockProvisioner()).checkOnce(ctx)
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service ไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceRunning {
		t.Errorf("รีสตาร์ทแล้วสถานะต้องคงเดิม แต่กลายเป็น %q (reason=%q) — "+
			"ความจำเสื่อมของ mock ไม่ใช่หลักฐานว่า workload หาย",
			after.Status, after.StatusReason)
	}
}

// TestHealthMonitorReportsCrashLoopEndToEnd — พิสูจน์ทั้งเส้นตั้งแต่ deploy จนสถานะถูกแก้
//
// นี่คือกติกาหลักของฟีเจอร์นี้: deploy ที่ "ผ่าน" ต้องไม่ถูกเขียนเป็น running ทันที
// แล้ว monitor ต้องเป็นคนเขียนสถานะจริงลงไปพร้อมเหตุผลที่ผู้ใช้เอาไปแก้ต่อได้
//
// ต้องมี Postgres — ไม่มีก็ข้ามไป (เหมือนเทสต์ integration ตัวอื่น)
func TestHealthMonitorReportsCrashLoopEndToEnd(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "health-monitor-test-ns")
	ctx := context.Background()

	// stub ที่บังคับให้คลัสเตอร์ "ตอบ" ว่ากำลังเตรียม container อยู่
	prov := &stubProv{
		MockProvisioner: NewMockProvisioner(),
		ns:              ns.Name,
		status: WorkloadStatus{
			Phase: PhasePending, Reason: "ContainerCreating", Message: "กำลังเตรียม container",
		},
	}
	mgr := NewServiceManager(db, NewQuotaService(db), prov)
	monitor := NewServiceHealthMonitor(db, prov)

	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "will-crash", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
		ContainerPort: 5432, IsDatabase: true, DataPath: "/var/lib/postgresql/data",
	})
	if err != nil {
		t.Fatalf("สร้าง service ไม่สำเร็จ: %v", err)
	}

	// ข้อสำคัญที่สุด: deploy ผ่านแล้วแต่ต้องยังไม่ใช่ running
	// เพราะคลัสเตอร์แค่ "รับคำสั่ง" ไม่ได้แปลว่า container ขึ้นได้
	if svc.Status == entity.ServiceRunning {
		t.Fatal("deploy ผ่านต้องไม่เขียน running ทันที — ต้องรอ monitor ยืนยันกับคลัสเตอร์ก่อน")
	}

	// รอบแรก: คลัสเตอร์บอกว่ายังเตรียม container อยู่ → ต้องได้ pending ไม่ใช่ running
	monitor.checkOnce(ctx)
	var after entity.Service
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service กลับมาไม่ได้: %v", err)
	}
	if after.Status != entity.ServicePending {
		t.Errorf("ช่วงเตรียม container ต้องเป็น %q ได้ %q", entity.ServicePending, after.Status)
	}

	// คลัสเตอร์เปลี่ยนคำตอบเป็น container ตายซ้ำๆ พร้อม log ที่บอกสาเหตุ
	prov.status = WorkloadStatus{
		Phase: PhaseCrashLoop, Reason: "CrashLoopBackOff", Restarts: 4,
		Message: "container ออกด้วย exit code 1\n" +
			"  error: database is uninitialized and superuser password is not specified",
	}

	// รอบสอง: ต้องได้ crashloop พร้อมเหตุผลและ log
	monitor.checkOnce(ctx)
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service กลับมาไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceCrashLoop {
		t.Fatalf("ต้องเป็น %q ได้ %q", entity.ServiceCrashLoop, after.Status)
	}
	if after.StatusReason == "" {
		t.Error("ต้องบันทึกเหตุผลไว้ ไม่งั้นผู้ใช้เห็นแค่ว่าพังโดยไม่รู้ว่าต้องแก้อะไร")
	}
	if after.StatusMessage == "" {
		t.Error("ต้องบันทึก log ท้ายๆ ก่อนตายไว้ — pod ถูกสร้างใหม่เรื่อยๆ log ตัวจริงจะหายไปก่อน")
	}
	if after.RestartCount < 1 {
		t.Errorf("restart_count = %d ต้องมากกว่า 0", after.RestartCount)
	}
	if after.StatusCheckedAt == nil {
		t.Error("ต้องบันทึกเวลาที่ถามคลัสเตอร์ล่าสุดไว้")
	}

	// หายดีแล้วต้องกลับเป็น running และล้างข้อความเก่าทิ้ง
	// (จำลองผู้ใช้แก้ค่าถูกแล้ว container ขึ้นได้)
	prov.status = WorkloadStatus{Phase: PhaseRunning}
	monitor.checkOnce(ctx)
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service กลับมาไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceRunning {
		t.Errorf("แก้แล้วต้องกลับเป็น %q ได้ %q", entity.ServiceRunning, after.Status)
	}
	if after.StatusReason != "" || after.StatusMessage != "" {
		t.Errorf("กลับมารันปกติแล้วต้องล้างข้อความเก่าทิ้ง เหลือ reason=%q message=%q",
			after.StatusReason, after.StatusMessage)
	}
}

// TestMonitorKeepsStatusWhenClusterUnreachable — ถามไม่ได้ ≠ ของพัง
//
// คลัสเตอร์สะดุดหรือเน็ตมีปัญหาชั่วคราวต้องไม่ทำให้ service ที่รันอยู่ดีๆ กลายเป็น failed
// ทั้งกระดาน ซึ่งจะทำให้ผู้ใช้ทั้งระบบตกใจพร้อมกันโดยไม่มีอะไรพังจริง
func TestMonitorKeepsStatusWhenClusterUnreachable(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "health-unreachable-test-ns")
	ctx := context.Background()

	prov := &stubProv{MockProvisioner: NewMockProvisioner(), ns: ns.Name,
		status: WorkloadStatus{Phase: PhaseRunning}}
	mgr := NewServiceManager(db, NewQuotaService(db), prov)
	monitor := NewServiceHealthMonitor(db, prov)

	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "steady", Image: "nginx:latest", CPUMilli: 300, RAMMB: 256,
	})
	if err != nil {
		t.Fatalf("สร้าง service ไม่สำเร็จ: %v", err)
	}

	monitor.checkOnce(ctx)
	var after entity.Service
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service ไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceRunning {
		t.Fatalf("ต้องเป็น running ก่อน ได้ %q", after.Status)
	}

	// ทำให้ถามคลัสเตอร์ไม่ได้ แล้วเช็คซ้ำ — สถานะต้องคงเดิม ไม่ใช่กลายเป็น failed
	monitor.prov = unreachableProv{prov.MockProvisioner}
	monitor.checkOnce(ctx)
	if err := db.First(&after, svc.ID).Error; err != nil {
		t.Fatalf("อ่าน service ไม่ได้: %v", err)
	}
	if after.Status != entity.ServiceRunning {
		t.Errorf("ถามคลัสเตอร์ไม่ได้ต้องคงสถานะเดิมไว้ แต่กลายเป็น %q", after.Status)
	}
}

// unreachableProv จำลองคลัสเตอร์ที่ถามไม่ได้ (ต่างจาก PhaseGone ที่แปลว่าถามได้แล้วของไม่อยู่)
type unreachableProv struct{ *MockProvisioner }

func (unreachableProv) Status(context.Context, string, *entity.Service) (WorkloadStatus, error) {
	return WorkloadStatus{}, errors.New("ต่อคลัสเตอร์ไม่ได้")
}
