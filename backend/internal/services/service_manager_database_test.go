package services

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// เทสต์ชุดนี้เดินเส้นทางจริงทั้งเส้น (ตรวจคำขอ → เช็คโควตา → INSERT → provisioner)
// จึงต้องมี Postgres เหมือน service_manager_rollback_test.go — ไม่มี DB ก็ข้ามไป ไม่ทำให้ CI พัง
//
// รันได้ด้วย: docker compose up -d db  แล้ว  go test ./internal/services/

// dbTestNamespace สร้าง namespace สะอาดๆ หนึ่งอันพร้อมเก็บกวาดให้เอง
func dbTestNamespace(t *testing.T, db *gorm.DB, name string) *entity.Namespace {
	t.Helper()
	root := context.Background()

	ns := &entity.Namespace{
		Name: name, ContributorID: 1,
		CPULimitMilli: entity.DefaultCPULimitMilli,
		RAMLimitMB:    entity.DefaultRAMLimitMB,
		// ปล่อยให้ StorageLimitMB ใช้ default ของคอลัมน์ (GORM ข้าม zero value ที่มี default tag)
		// ซึ่งเป็นสถานการณ์เดียวกับ namespace เดิมที่ผ่าน AutoMigrate มา
	}
	db.WithContext(root).Where("name = ?", name).Delete(&entity.Namespace{})
	if err := db.WithContext(root).Create(ns).Error; err != nil {
		t.Skipf("ข้าม: สร้าง namespace ทดสอบไม่ได้ (%v)", err)
	}
	t.Cleanup(func() {
		db.WithContext(root).Where("namespace_id = ?", ns.ID).Delete(&entity.Service{})
		db.WithContext(root).Delete(&entity.Namespace{}, ns.ID)
	})
	return ns
}

// TestCreateDatabaseAppliesRules — สวิตช์เดียวต้องเปลี่ยนครบทั้งชุด
//
// ถ้าข้อใดข้อหนึ่งหลุด อาการจะต่างกันคนละแบบและทุกแบบ "ดูเหมือนสำเร็จ":
// ไม่มี storage = ข้อมูลหาย, ได้ NodePort = เปิด database ออกทั้งเครือข่าย,
// replicas ไม่ตรึง = สอง Pod เขียนดิสก์ก้อนเดียวกัน
//
// ใช้ image อะไรก็ได้ ระบบไม่แยกชนิดของ database — ทุกอย่างมาจากที่ผู้ใช้กรอก
func TestCreateDatabaseAppliesRules(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-rules-test-ns")
	ctx := context.Background()

	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())
	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name:          "my-pg",
		Image:         "postgres:16",
		CPUMilli:      500,
		RAMMB:         512,
		ContainerPort: 5432,
		IsDatabase:    true,
		DataPath:      "/var/lib/postgresql/data",
		EnvVars:       map[string]string{"POSTGRES_PASSWORD": "s3cret", "POSTGRES_DB": "app"},
	})
	if err != nil {
		t.Fatalf("สร้าง database ไม่สำเร็จ: %v", err)
	}

	if svc.ContainerPort != 5432 {
		t.Errorf("ContainerPort = %d ต้องใช้ค่าที่ผู้ใช้กรอก 5432", svc.ContainerPort)
	}
	if svc.DataPath != "/var/lib/postgresql/data" {
		t.Errorf("DataPath = %q ไม่ตรงกับที่กรอกมา", svc.DataPath)
	}
	if svc.Replicas != entity.StorageReplicas {
		t.Errorf("Replicas = %d ต้องถูกตรึงที่ %d", svc.Replicas, entity.StorageReplicas)
	}
	if svc.StorageMB != entity.DefaultStorageMBPerService {
		t.Errorf("StorageMB = %d ไม่ได้ระบุมาต้องได้ default %d",
			svc.StorageMB, entity.DefaultStorageMBPerService)
	}
	// ข้อสำคัญที่สุด — NodePort ที่ถูกจ่ายไปคือประตูที่เปิดให้ทั้งเครือข่ายมหาวิทยาลัยเข้าถึง database
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่มี NodePort แต่ได้ %d", *svc.NodePort)
	}
	// env ต้องถูกส่งต่อตามที่กรอกมาเป๊ะๆ ระบบไม่เติมและไม่ตัดอะไรทั้งนั้น
	if svc.EnvVars["POSTGRES_PASSWORD"] != "s3cret" || len(svc.EnvVars) != 2 {
		t.Errorf("env ต้องเป็นตามที่ผู้ใช้กรอกเท่านั้น ได้ %v", svc.EnvVars)
	}

	// โควตาดิสก์ต้องถูกหักจริง ไม่ใช่แค่เก็บตัวเลขไว้เฉยๆ
	usage, err := NewQuotaService(db).Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.UsedStorageMB != entity.DefaultStorageMBPerService {
		t.Errorf("UsedStorageMB = %d ต้องเป็น %d", usage.UsedStorageMB, entity.DefaultStorageMBPerService)
	}
}

// TestCreateDatabaseAcceptsAnyImage — image อะไรก็ใช้เป็น database ได้
//
// ของที่สวิตช์ database ให้จริงๆ (ClusterIP + NetworkPolicy + PVC + 1 pod) ไม่มีข้อไหน
// ต้องรู้ว่าเป็น database ชนิดไหน คนที่ build image เองจึงต้องไม่ถูกกันออก
func TestCreateDatabaseAcceptsAnyImage(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-custom-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "my-custom-db", Image: "ghcr.io/lab/custom-db:1",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 9042,
		IsDatabase: true, DataPath: "/var/lib/customdb/",
		EnvVars: map[string]string{"ANYTHING": "goes"},
	})
	if err != nil {
		t.Fatalf("image ที่ระบบไม่เคยเห็นต้อง deploy เป็น database ได้: %v", err)
	}

	// ตัด / ท้ายทิ้งให้ เพื่อไม่ให้ "/var/lib/db" กับ "/var/lib/db/" กลายเป็นคนละค่าใน DB
	if svc.DataPath != "/var/lib/customdb" {
		t.Errorf("DataPath = %q ต้องเป็น /var/lib/customdb (ตัด / ท้ายออก)", svc.DataPath)
	}
	if svc.Replicas != entity.StorageReplicas {
		t.Errorf("Replicas = %d ต้องถูกตรึงที่ %d", svc.Replicas, entity.StorageReplicas)
	}
	if svc.NodePort != nil {
		t.Errorf("ต้องไม่ได้ NodePort แต่ได้ %d — เท่ากับเปิดออกนอกคลัสเตอร์", *svc.NodePort)
	}
	if svc.ContainerPort != 9042 {
		t.Errorf("ContainerPort = %d ต้องใช้ค่าที่ผู้ใช้กรอก 9042", svc.ContainerPort)
	}
	// ไม่ตรวจ env ให้เลย ผู้ใช้ใส่อะไรมาก็ผ่าน (ถ้ากรอกไม่ครบไปรู้ผลที่หน้า Logs แทน)
	if svc.EnvVars["ANYTHING"] != "goes" {
		t.Error("env ที่ผู้ใช้กรอกต้องถูกส่งต่อไปตามเดิม")
	}
}

// TestCreateWebWithStorage — web ขอดิสก์ถาวรได้โดยไม่ต้องเปิดสวิตช์ database (docs 022)
//
// เคสจริง: Nextcloud เก็บไฟล์ผู้ใช้ใน /var/www/html — ไม่มีดิสก์ ไฟล์อยู่บนดิสก์ชั่วคราวของ node
// หายตอน pod ถูกสร้างใหม่ แต่ถ้าไปเปิดสวิตช์ database แทน จะได้ดิสก์แต่คนนอกเข้าแอปไม่ได้ (ไม่มี NodePort)
func TestCreateWebWithStorage(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "web-disk-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	svc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "nextcloud", Image: "nextcloud:apache",
		CPUMilli: 500, RAMMB: 512, ContainerPort: 80,
		StorageMB: 2048, DataPath: "/var/www/html/",
		EnvVars: map[string]string{"MYSQL_HOST": "mariadb"},
	})
	if err != nil {
		t.Fatalf("สร้าง web ที่มีดิสก์ไม่สำเร็จ: %v", err)
	}

	if svc.IsDatabase {
		t.Error("ต้องไม่ถูกเปลี่ยนเป็น database")
	}
	if !svc.HasStorage() || svc.StorageMB != 2048 {
		t.Errorf("StorageMB = %d ต้องเป็น 2048 ตามที่ขอ", svc.StorageMB)
	}
	if svc.DataPath != "/var/www/html" {
		t.Errorf("DataPath = %q ต้องเป็น /var/www/html (ตัด / ท้ายออก)", svc.DataPath)
	}
	if svc.Replicas != entity.StorageReplicas {
		t.Errorf("Replicas = %d ต้องถูกตรึงที่ %d", svc.Replicas, entity.StorageReplicas)
	}
	if svc.NodePort == nil {
		t.Error("web ที่มีดิสก์ต้องได้ NodePort — ไม่งั้นคนนอกเข้าใช้แอปไม่ได้")
	}

	usage, err := NewQuotaService(db).Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.UsedStorageMB != 2048 {
		t.Errorf("UsedStorageMB = %d ต้องเป็น 2048 (ดิสก์ของ web ต้องหักโควตาเหมือน database)", usage.UsedStorageMB)
	}
}

// TestCreateDatabaseRejectsBadRequests — ทุกเคสต้องคืน error ที่ระบุสาเหตุได้
// เพราะ controller แปลงเป็น error code คนละตัวให้หน้าเว็บจัดการต่างกัน
func TestCreateDatabaseRejectsBadRequests(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-reject-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	cases := []struct {
		name   string
		params CreateServiceParams
		want   error
	}{
		{
			name: "ไม่บอกจุดเก็บข้อมูล",
			params: CreateServiceParams{
				Name: "nopath", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
				IsDatabase: true,
			},
			want: ErrDataPathRequired,
		},
		{
			name: "จุดเก็บข้อมูลไม่ใช่ absolute path",
			params: CreateServiceParams{
				Name: "badpath", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
				IsDatabase: true, DataPath: "data/mydb",
			},
			want: ErrDataPathRequired,
		},
		{
			name: "mount ทับโฟลเดอร์ระบบ",
			params: CreateServiceParams{
				Name: "syspath", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
				IsDatabase: true, DataPath: "/usr/local/data",
			},
			want: ErrDataPathRequired,
		},
		{
			name: "ขอหลาย replica ให้ database",
			params: CreateServiceParams{
				Name: "pg-many", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
				IsDatabase: true, DataPath: "/var/lib/postgresql/data", Replicas: 3,
			},
			want: ErrStorageReplicas,
		},
		// web ขอดิสก์ได้แล้ว (docs 022) แต่ต้องผ่านกติกาเดียวกับ database ทุกข้อ
		{
			name: "web ขอดิสก์แต่ไม่บอกจุดเก็บข้อมูล",
			params: CreateServiceParams{
				Name: "web-disk", Image: "nextcloud:apache", CPUMilli: 300, RAMMB: 256,
				StorageMB: 4096,
			},
			want: ErrDataPathRequired,
		},
		{
			name: "web ที่มีดิสก์ขอหลาย replica",
			params: CreateServiceParams{
				Name: "web-disk-many", Image: "nextcloud:apache", CPUMilli: 300, RAMMB: 256,
				StorageMB: 4096, DataPath: "/var/www/html", Replicas: 3,
			},
			want: ErrStorageReplicas,
		},
		{
			name: "web ส่งแค่ data_path ก็ถือว่าขอดิสก์ ต้องผ่านกติกา path",
			params: CreateServiceParams{
				Name: "web-badpath", Image: "nextcloud:apache", CPUMilli: 300, RAMMB: 256,
				DataPath: "/etc/nextcloud",
			},
			want: ErrDataPathRequired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mgr.Create(ctx, 1, ns.ID, tc.params)
			if !errors.Is(err, tc.want) {
				t.Errorf("ได้ error %v ต้องเป็น %v", err, tc.want)
			}
		})
	}

	// ไม่มีคำขอไหนผ่านเลย = ต้องไม่มีแถวค้างกินโควตาอยู่
	usage, err := NewQuotaService(db).Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.ServiceCount != 0 {
		t.Errorf("คำขอที่ถูกปฏิเสธต้องไม่ทิ้งแถวไว้ แต่เหลือ %d service", usage.ServiceCount)
	}
}

// TestScaleWithStorageIsRejected — หน้าเว็บซ่อน dropdown ให้แล้ว แต่เส้น PATCH เรียกตรงได้
// ด่านจริงต้องอยู่ที่ service layer ไม่ใช่ที่ UI — กันทุก service ที่มีดิสก์ ไม่ใช่แค่ database
func TestScaleWithStorageIsRejected(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-scale-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	for _, p := range []CreateServiceParams{
		{Name: "pg-scale", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
			IsDatabase: true, DataPath: "/var/lib/postgresql/data"},
		{Name: "nc-scale", Image: "nextcloud:apache", CPUMilli: 300, RAMMB: 256, ContainerPort: 80,
			StorageMB: 2048, DataPath: "/var/www/html"},
	} {
		svc, err := mgr.Create(ctx, 1, ns.ID, p)
		if err != nil {
			t.Fatalf("สร้าง %s ไม่สำเร็จ: %v", p.Name, err)
		}
		if _, err := mgr.Scale(ctx, svc.ID, ns.ID, 3); !errors.Is(err, ErrStorageReplicas) {
			t.Errorf("scale %s ต้องถูกปฏิเสธด้วย ErrStorageReplicas ได้ %v", p.Name, err)
		}
	}
}

// TestDeleteWithStorageReleasesStorage ยืนยันว่าโควตาดิสก์คืนครบหลังลบ ทั้ง database และ web ที่มีดิสก์
// (บน mock ไม่มี PVC จริง แต่ทดสอบได้ว่าฝั่ง DB ของเราคิดเลขถูก)
func TestDeleteWithStorageReleasesStorage(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-delete-test-ns")
	ctx := context.Background()
	quota := NewQuotaService(db)
	mgr := NewServiceManager(db, quota, NewMockProvisioner())

	var created []*entity.Service
	for _, p := range []CreateServiceParams{
		{Name: "pg-del", Image: "postgres:16", CPUMilli: 300, RAMMB: 256,
			IsDatabase: true, StorageMB: 2048, DataPath: "/var/lib/postgresql/data"},
		{Name: "nc-del", Image: "nextcloud:apache", CPUMilli: 300, RAMMB: 256, ContainerPort: 80,
			StorageMB: 3072, DataPath: "/var/www/html"},
	} {
		svc, err := mgr.Create(ctx, 1, ns.ID, p)
		if err != nil {
			t.Fatalf("สร้าง %s ไม่สำเร็จ: %v", p.Name, err)
		}
		created = append(created, svc)
	}

	usage, err := quota.Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.UsedStorageMB != 2048+3072 {
		t.Errorf("ก่อนลบ UsedStorageMB = %d ต้องเป็น %d (ดิสก์ของ web ต้องถูกหักด้วย)", usage.UsedStorageMB, 2048+3072)
	}

	for _, svc := range created {
		if err := mgr.Delete(ctx, svc.ID, ns.ID); err != nil {
			t.Fatalf("ลบ %s ไม่สำเร็จ: %v", svc.Name, err)
		}
	}

	usage, err = quota.Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.UsedStorageMB != 0 || usage.ServiceCount != 0 {
		t.Errorf("ลบแล้วต้องคืนโควตาครบ แต่เหลือ %d service กินดิสก์ %d MB",
			usage.ServiceCount, usage.UsedStorageMB)
	}
}

// TestValidateDataPath — ด่านเดียวที่กันไม่ให้จุด mount ที่ผู้ใช้กรอกทำให้ container พัง
//
// ตรวจได้แค่ว่า path นี้ mount แล้วไม่พัง ตรวจไม่ได้ว่าตรงกับที่ image เขียนข้อมูลจริงไหม
// อันหลังต้องพึ่งผู้ใช้ ซึ่งเป็นเหตุผลที่ฟอร์มต้องเตือนให้ชัดว่ากรอกผิดแล้วข้อมูลจะไม่ถูกเก็บ
func TestValidateDataPath(t *testing.T) {
	ok := []string{
		"/var/lib/mydb",
		"/data",
		"/opt/app/storage",
		"/var/lib/mydb/", // / ท้ายยอมรับได้ ผู้เรียกตัดทิ้งเอง
		"/home/db",
	}
	for _, p := range ok {
		if msg := ValidateDataPath(p); msg != "" {
			t.Errorf("ValidateDataPath(%q) ต้องผ่าน แต่ได้ %q", p, msg)
		}
	}

	bad := []string{
		"",                // ไม่กรอก
		"   ",             // ช่องว่างล้วน
		"data/mydb",       // relative
		"/var/../etc",     // หนีออกด้วย ..
		"/var//lib",       // // ติดกัน
		"/",               // ทับ root ทั้งก้อน
		"/etc",            // โฟลเดอร์ระบบ
		"/usr/local/data", // โฟลเดอร์ย่อยของระบบ
		"/proc/self",      // ของ kernel
	}
	for _, p := range bad {
		if msg := ValidateDataPath(p); msg == "" {
			t.Errorf("ValidateDataPath(%q) ต้องไม่ผ่าน", p)
		}
	}

	// ชื่อที่ขึ้นต้นคล้ายโฟลเดอร์ระบบแต่คนละตัว ต้องไม่โดนบล็อกผิด
	for _, p := range []string{"/usrdata", "/etcetera/db", "/devices/db"} {
		if msg := ValidateDataPath(p); msg != "" {
			t.Errorf("ValidateDataPath(%q) ต้องผ่าน (ไม่ใช่โฟลเดอร์ระบบ) แต่ได้ %q", p, msg)
		}
	}
}

// TestMockDeployDatabaseNeverAssignsNodePort คือกติกาข้อสำคัญที่สุดของฟีเจอร์นี้
//
// NodePort ที่ถูกจ่ายไปแล้วคือประตูที่เปิดให้ทั้งเครือข่ายมหาวิทยาลัยยิงเข้า database ได้
// ต่อให้ NetworkPolicy ถูกต้อง การไม่จองพอร์ตตั้งแต่แรกคือชั้นป้องกันที่เชื่อถือได้กว่า
// เทสต์นี้จับได้ตั้งแต่ตอนรัน mock โดยไม่ต้องมีคลัสเตอร์จริง
func TestMockDeployDatabaseNeverAssignsNodePort(t *testing.T) {
	m := NewMockProvisioner()
	ctx := context.Background()

	db := &entity.Service{
		ID: 1, Name: "my-db", Image: "postgres:16", IsDatabase: true,
		CPUMilli: 500, RAMMB: 512, ContainerPort: 5432,
		Replicas: entity.StorageReplicas, StorageMB: entity.DefaultStorageMBPerService,
		DataPath: "/var/lib/postgresql/data",
	}
	if err := m.DeployService(ctx, "ns-user-12", db); err != nil {
		t.Fatalf("deploy database ไม่ควรล้มเหลวบน mock: %v", err)
	}
	if db.NodePort != nil {
		t.Errorf("database ต้องไม่ได้ NodePort แต่ได้ %d — เท่ากับเปิดออกนอกคลัสเตอร์", *db.NodePort)
	}

	// service ธรรมดายังต้องได้ NodePort เหมือนเดิม ไม่ใช่ปิดไปทั้งระบบ
	app := &entity.Service{
		ID: 2, Name: "web", Image: "nginx:latest",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 8080, Replicas: 1,
	}
	if err := m.DeployService(ctx, "ns-user-12", app); err != nil {
		t.Fatalf("deploy service ธรรมดาไม่ควรล้มเหลว: %v", err)
	}
	if app.NodePort == nil {
		t.Error("service ธรรมดาต้องยังได้ NodePort เหมือนเดิม")
	}

	// web ที่มีดิสก์ (แบบ Nextcloud) ต้องได้ NodePort — ดิสก์ถาวรไม่ได้แปลว่าต้องปิดจากคนนอก (docs 022)
	webDisk := &entity.Service{
		ID: 3, Name: "nextcloud", Image: "nextcloud:apache",
		CPUMilli: 500, RAMMB: 512, ContainerPort: 80, Replicas: entity.StorageReplicas,
		StorageMB: entity.DefaultStorageMBPerService, DataPath: "/var/www/html",
	}
	if err := m.DeployService(ctx, "ns-user-12", webDisk); err != nil {
		t.Fatalf("deploy web ที่มีดิสก์ไม่ควรล้มเหลว: %v", err)
	}
	if webDisk.NodePort == nil {
		t.Error("web ที่มีดิสก์ต้องได้ NodePort — ไม่งั้นคนนอกเข้าใช้แอปไม่ได้")
	}
}

// TestUpdateWithStorageKeepsDisk — แก้ image ของ service ที่มีดิสก์ผ่าน ServiceManager ต้องผ่าน
// และแก้ค่าที่ผูกกับ PVC ต้องได้ ErrStorageImmutable โดยแถวใน DB ไม่ถูกแก้ (docs 030)
func TestUpdateWithStorageKeepsDisk(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "update-disk-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	created, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{
		Name: "nextcloud", Image: "nextcloud:apache",
		CPUMilli: 500, RAMMB: 512, ContainerPort: 80,
		StorageMB: 2048, DataPath: "/var/www/html",
	})
	if err != nil {
		t.Fatalf("สร้าง web ที่มีดิสก์ไม่สำเร็จ: %v", err)
	}

	params := UpdateServiceParams{
		Name: "nextcloud", Image: "nextcloud:31-apache",
		CPUMilli: 500, RAMMB: 512, ContainerPort: 80, Replicas: 1,
		StorageMB: 2048, DataPath: "/var/www/html",
	}
	updated, err := mgr.Update(ctx, created.ID, 1, ns.ID, params)
	if err != nil {
		t.Fatalf("แก้ image ต้องผ่าน: %v", err)
	}
	if updated.NodePort == nil || created.NodePort == nil || *updated.NodePort != *created.NodePort {
		t.Errorf("NodePort ต้องคงเดิม เดิม=%v ใหม่=%v", created.NodePort, updated.NodePort)
	}

	bad := params
	bad.Name = "nextcloud-new"
	if _, err := mgr.Update(ctx, created.ID, 1, ns.ID, bad); !errors.Is(err, ErrStorageImmutable) {
		t.Fatalf("เปลี่ยนชื่อ service ที่มีดิสก์ต้องได้ ErrStorageImmutable ได้ %v", err)
	}
	var row entity.Service
	if err := db.WithContext(ctx).First(&row, created.ID).Error; err != nil {
		t.Fatalf("อ่านแถวไม่สำเร็จ: %v", err)
	}
	if row.Name != "nextcloud" || row.Image != "nextcloud:31-apache" {
		t.Errorf("คำขอที่ถูกปฏิเสธต้องไม่แก้ DB ได้ name=%q image=%q", row.Name, row.Image)
	}
}
