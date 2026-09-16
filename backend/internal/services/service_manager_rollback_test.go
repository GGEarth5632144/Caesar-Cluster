package services

import (
	"context"
	"io"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/entity"
)

// เทสต์ในไฟล์นี้ต้องมี Postgres จริง เพราะสิ่งที่ตรวจคือพฤติกรรมของ transaction + context
// ที่จำลองด้วย sqlite/mock ไม่ได้ (context.Canceled ถูกตรวจที่ชั้น driver) — ไม่มี DB ก็ข้ามไป
// ไม่ทำให้ CI ที่ไม่มี DB พังทั้งชุด
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5433/cloud_cluster?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("ข้าม: ต่อ DB ทดสอบไม่ได้ (%v) — ตั้ง TEST_DB_URL ถ้าใช้ที่อื่น", err)
	}

	// migrate สองตารางที่เทสต์ชุดนี้ใช้ก่อนเสมอ
	//
	// จำเป็นเพราะ AutoMigrate ของจริงรันแค่ใน cmd/server กับ cmd/seed — เทสต์ต่อ DB ตรงๆ
	// ถ้า dev เคยรันเซิร์ฟเวอร์เวอร์ชันเก่าค้างไว้ ตารางจะไม่มีคอลัมน์ที่เพิ่งเพิ่ม แล้วเทสต์
	// จะล้มด้วย "column does not exist" ซึ่งดูเหมือนโค้ดพังทั้งที่แค่ schema ตามไม่ทัน
	if err := db.AutoMigrate(&entity.Namespace{}, &entity.Service{}); err != nil {
		t.Skipf("ข้าม: migrate schema ของ DB ทดสอบไม่สำเร็จ (%v)", err)
	}
	return db
}

// disconnectingProv จำลอง "ผู้ใช้ปิดหน้าเว็บระหว่างรอ deploy":
// context ของ HTTP request ถูก cancel ก่อน แล้ว provisioner จึงล้มเหลวเพราะ context นั้นเอง
type disconnectingProv struct{ cancel context.CancelFunc }

func (p *disconnectingProv) EnsureNamespace(context.Context, *entity.Namespace) error     { return nil }
func (p *disconnectingProv) DeleteNamespace(context.Context, string) error                { return nil }
func (p *disconnectingProv) DeleteService(context.Context, string, *entity.Service) error { return nil }
func (p *disconnectingProv) ScaleService(context.Context, string, string, int) error      { return nil }
func (p *disconnectingProv) UpdateService(context.Context, string, *entity.Service, *entity.Service) error {
	return nil
}
func (p *disconnectingProv) Logs(context.Context, string, string, LogOptions) (io.ReadCloser, error) {
	return nil, nil
}
func (p *disconnectingProv) Status(context.Context, string, *entity.Service) (WorkloadStatus, error) {
	return WorkloadStatus{Phase: PhaseRunning}, nil
}
func (p *disconnectingProv) DatabaseCredentials(context.Context, string, *entity.Service) (DatabaseCredentials, error) {
	return DatabaseCredentials{}, nil
}
func (p *disconnectingProv) DeployService(ctx context.Context, _ string, _ *entity.Service) error {
	p.cancel()
	return ctx.Err()
}

// TestCreateReleasesQuotaWhenClientDisconnects — regression test ของบั๊กโควตารั่ว
//
// เดิม Create ถอย row ที่จองโควตาไว้ด้วย ctx ตัวเดียวกับที่เพิ่งถูก cancel คำสั่ง DELETE
// จึงล้มเหลวทันทีด้วย "context canceled" และ error ก้อนนั้นถูกทิ้ง ผลคือแถว status=creating
// ค้างกินโควตาไปตลอดโดยไม่มีใครรู้ (วัดได้จริง: หายไป 400m CPU / 256 MB ต่อการกดหนึ่งครั้ง)
//
// ผู้ใช้กดปุ่ม deploy แล้วเปลี่ยนหน้าทันทีเป็นเรื่องปกติมาก บั๊กนี้จึงไม่ใช่เคสมุมอับ
func TestCreateReleasesQuotaWhenClientDisconnects(t *testing.T) {
	db := testDB(t)
	root := context.Background()

	ns := &entity.Namespace{
		Name: "rollback-test-ns", ContributorID: 1,
		CPULimitMilli: entity.DefaultCPULimitMilli, RAMLimitMB: entity.DefaultRAMLimitMB,
	}
	db.WithContext(root).Where("name = ?", ns.Name).Delete(&entity.Namespace{})
	if err := db.WithContext(root).Create(ns).Error; err != nil {
		t.Skipf("ข้าม: สร้าง namespace ทดสอบไม่ได้ (%v)", err)
	}
	t.Cleanup(func() {
		db.WithContext(root).Where("namespace_id = ?", ns.ID).Delete(&entity.Service{})
		db.WithContext(root).Delete(&entity.Namespace{}, ns.ID)
	})

	ctx, cancel := context.WithCancel(root)
	t.Cleanup(cancel)
	mgr := NewServiceManager(db, NewQuotaService(db), &disconnectingProv{cancel: cancel})

	if _, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "disconnect-probe", Image: "nginx:1.27-alpine", CPUMilli: 400, RAMMB: 256,
	}); err == nil {
		t.Fatal("deploy ควรล้มเหลวเมื่อ context ถูก cancel")
	}

	usage, err := NewQuotaService(db).Usage(root, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.ServiceCount != 0 || usage.UsedCPUMilli != 0 || usage.UsedRAMMB != 0 {
		t.Errorf("deploy ล้มเหลวแล้วต้องคืนโควตาครบ แต่ยังเหลือ %d service กิน %dm CPU / %d MB",
			usage.ServiceCount, usage.UsedCPUMilli, usage.UsedRAMMB)
	}
}
