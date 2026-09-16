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

// TestCreateDatabaseAppliesRules — database จาก template ต้องได้กติกาครบทั้งชุดจาก catalog
//
// ถ้าข้อใดข้อหนึ่งหลุด อาการจะต่างกันคนละแบบและทุกแบบ "ดูเหมือนสำเร็จ":
// ไม่มี storage = ข้อมูลหาย, ได้ NodePort = เปิด database ออกทั้งเครือข่าย,
// replicas ไม่ตรึง = สอง Pod เขียนดิสก์ก้อนเดียวกัน, รหัสลง DB = รหัสรั่วไปกับ backup ของ backend
func TestCreateDatabaseAppliesRules(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-rules-test-ns")
	ctx := context.Background()

	prov := NewMockProvisioner()
	mgr := NewServiceManager(db, NewQuotaService(db), prov)
	svc, err := mgr.CreateDatabase(ctx, 1, ns.ID, CreateDatabaseParams{
		Engine: EnginePostgreSQL, Version: "17",
		Name: "my-pg", Username: "appuser", Password: "p@ss:w/rd#1", Database: "appdb",
		CPUMilli: 500, RAMMB: 512,
	})
	if err != nil {
		t.Fatalf("สร้าง database ไม่สำเร็จ: %v", err)
	}

	if svc.Image != "postgres:17" || svc.ContainerPort != 5432 || svc.DataPath != "/var/lib/postgresql/data" {
		t.Errorf("image/port/data_path ต้องมาจาก catalog ได้ %q %d %q", svc.Image, svc.ContainerPort, svc.DataPath)
	}
	if !svc.IsDatabase || svc.DatabaseEngine != EnginePostgreSQL || svc.DatabaseVersion != "17" {
		t.Errorf("ต้องเป็น database template postgresql 17 ได้ is_database=%v engine=%q version=%q",
			svc.IsDatabase, svc.DatabaseEngine, svc.DatabaseVersion)
	}
	if svc.Replicas != entity.StorageReplicas {
		t.Errorf("Replicas = %d ต้องถูกตรึงที่ %d", svc.Replicas, entity.StorageReplicas)
	}
	if svc.StorageMB != entity.DefaultStorageMBPerService {
		t.Errorf("StorageMB = %d ไม่ได้ระบุมาต้องได้ default %d", svc.StorageMB, entity.DefaultStorageMBPerService)
	}
	// ข้อสำคัญที่สุด — NodePort ที่ถูกจ่ายไปคือประตูที่เปิดให้ทั้งเครือข่ายมหาวิทยาลัยเข้าถึง database
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่มี NodePort แต่ได้ %d", *svc.NodePort)
	}
	if len(svc.EnvVars) != 0 {
		t.Errorf("database template ต้องไม่มี env ที่เป็นค่าจริง (อ่านจาก Secret) ได้ %v", svc.EnvVars)
	}
	// PostgreSQL ไม่ต้องใช้ root password — ห้ามสุ่มมาเก็บเปล่าๆ
	if svc.DBRootPassword != "" {
		t.Error("PostgreSQL ต้องไม่มี root password")
	}

	var row entity.Service
	if err := db.WithContext(ctx).First(&row, svc.ID).Error; err != nil {
		t.Fatalf("อ่านแถวไม่สำเร็จ: %v", err)
	}
	if row.DBUsername != "appuser" || row.DBName != "appdb" || row.DBPassword != "" {
		t.Errorf("DB ต้องเก็บ username/db แต่ไม่เก็บรหัส ได้ user=%q db=%q", row.DBUsername, row.DBName)
	}

	conn, err := mgr.Connection(ctx, svc.ID, ns.ID)
	if err != nil {
		t.Fatalf("อ่านข้อมูลการเชื่อมต่อไม่สำเร็จ: %v", err)
	}
	want := "postgresql://appuser:p%40ss%3Aw%2Frd%231@my-pg:5432/appdb?sslmode=disable"
	if conn.URL != want || conn.Password != "p@ss:w/rd#1" || conn.Host != "my-pg" {
		t.Errorf("connection ไม่ตรง ได้ url=%q host=%q", conn.URL, conn.Host)
	}

	usage, err := NewQuotaService(db).Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.UsedStorageMB != entity.DefaultStorageMBPerService {
		t.Errorf("UsedStorageMB = %d ต้องเป็น %d", usage.UsedStorageMB, entity.DefaultStorageMBPerService)
	}
}

// TestCreateDatabaseMySQLGetsRootPassword — MySQL/MariaDB image บังคับ root password: ระบบต้องสุ่มให้เอง
func TestCreateDatabaseMySQLGetsRootPassword(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-mysql-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	svc, err := mgr.CreateDatabase(ctx, 1, ns.ID, CreateDatabaseParams{
		Engine: EngineMariaDB, Name: "maria", Username: "appuser", Password: "secret123", Database: "appdb",
		CPUMilli: 300, RAMMB: 256, StorageMB: 2048,
	})
	if err != nil {
		t.Fatalf("สร้าง MariaDB ไม่สำเร็จ: %v", err)
	}
	if svc.DatabaseVersion != "11.8" || svc.Image != "mariadb:11.8" {
		t.Errorf("ไม่ระบุ version ต้องได้ default 11.8 ได้ %q (%s)", svc.DatabaseVersion, svc.Image)
	}
	if len(svc.DBRootPassword) < 32 {
		t.Errorf("root password ต้องถูกสุ่มให้ ได้ความยาว %d", len(svc.DBRootPassword))
	}
	if svc.StorageMB != 2048 {
		t.Errorf("StorageMB = %d ต้องเป็น 2048 ตามที่ขอ", svc.StorageMB)
	}
}

// TestCreateDatabaseRejectsBadRequests — ทุกเคสต้องคืน error ที่ระบุสาเหตุได้และไม่ทิ้งแถวกินโควตา
func TestCreateDatabaseRejectsBadRequests(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-reject-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	ok := CreateDatabaseParams{
		Engine: EnginePostgreSQL, Name: "pg", Username: "appuser", Password: "secret123", Database: "appdb",
		CPUMilli: 300, RAMMB: 256,
	}
	with := func(f func(*CreateDatabaseParams)) CreateDatabaseParams { p := ok; f(&p); return p }

	cases := []struct {
		name   string
		params CreateDatabaseParams
		want   error
	}{
		{"engine ที่ไม่มีใน catalog", with(func(p *CreateDatabaseParams) { p.Engine = "mongodb" }), ErrDatabaseEngineNotFound},
		{"version หมด support/ไม่มี", with(func(p *CreateDatabaseParams) { p.Version = "13" }), ErrDatabaseVersionNotFound},
		{"username เป็น root", with(func(p *CreateDatabaseParams) { p.Username = "root" }), ErrDatabaseInvalidInput},
		{"username ขึ้นต้น pg_", with(func(p *CreateDatabaseParams) { p.Username = "pg_app" }), ErrDatabaseInvalidInput},
		{"username ตัวพิมพ์ใหญ่", with(func(p *CreateDatabaseParams) { p.Username = "AppUser" }), ErrDatabaseInvalidInput},
		{"password สั้น", with(func(p *CreateDatabaseParams) { p.Password = "short" }), ErrDatabaseInvalidInput},
		{"password มีช่องว่าง", with(func(p *CreateDatabaseParams) { p.Password = "has space1" }), ErrDatabaseInvalidInput},
		{"password มี quote", with(func(p *CreateDatabaseParams) { p.Password = "it's-secret" }), ErrDatabaseInvalidInput},
		{"ชื่อ database ขึ้นต้นตัวเลข", with(func(p *CreateDatabaseParams) { p.Database = "1db" }), ErrDatabaseInvalidInput},
		{"ชื่อ database ของระบบ", with(func(p *CreateDatabaseParams) { p.Database = "template1" }), ErrDatabaseInvalidInput},
		{"MySQL RAM ต่ำกว่าขั้นต่ำ", with(func(p *CreateDatabaseParams) { p.Engine = EngineMySQL; p.RAMMB = 512 }), ErrDatabaseRAMTooLow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := mgr.CreateDatabase(ctx, 1, ns.ID, tc.params); !errors.Is(err, tc.want) {
				t.Errorf("ได้ error %v ต้องเป็น %v", err, tc.want)
			}
		})
	}

	usage, err := NewQuotaService(db).Usage(ctx, nil, ns.ID)
	if err != nil {
		t.Fatalf("อ่านยอดใช้งานไม่สำเร็จ: %v", err)
	}
	if usage.ServiceCount != 0 {
		t.Errorf("คำขอที่ถูกปฏิเสธต้องไม่ทิ้งแถวไว้ แต่เหลือ %d service", usage.ServiceCount)
	}
}

// TestUpdateTemplateDatabaseOnlyResources — database template แก้ได้แค่ CPU/RAM
func TestUpdateTemplateDatabaseOnlyResources(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-update-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	svc, err := mgr.CreateDatabase(ctx, 1, ns.ID, CreateDatabaseParams{
		Engine: EnginePostgreSQL, Name: "pg-upd", Username: "appuser", Password: "secret123", Database: "appdb",
		CPUMilli: 300, RAMMB: 256,
	})
	if err != nil {
		t.Fatalf("สร้าง database ไม่สำเร็จ: %v", err)
	}
	same := UpdateServiceParams{
		Name: svc.Name, Image: svc.Image, CPUMilli: 500, RAMMB: 512, ContainerPort: svc.ContainerPort,
		Replicas: 1, IsDatabase: true, StorageMB: svc.StorageMB, DataPath: svc.DataPath,
	}
	updated, err := mgr.Update(ctx, svc.ID, 1, ns.ID, same)
	if err != nil {
		t.Fatalf("แก้ CPU/RAM ต้องผ่าน: %v", err)
	}
	if updated.CPUMilli != 500 || updated.RAMMB != 512 || updated.DatabaseEngine != EnginePostgreSQL {
		t.Errorf("ค่าหลังแก้ไม่ตรง: %+v", updated)
	}

	for name, f := range map[string]func(*UpdateServiceParams){
		"เปลี่ยน image": func(p *UpdateServiceParams) { p.Image = "postgres:16" },
		"ใส่ env":       func(p *UpdateServiceParams) { p.EnvVars = map[string]string{"A": "b"} },
		"เปลี่ยนชื่อ":   func(p *UpdateServiceParams) { p.Name = "pg-new" },
	} {
		p := same
		f(&p)
		if _, err := mgr.Update(ctx, svc.ID, 1, ns.ID, p); !errors.Is(err, ErrDatabaseImmutable) {
			t.Errorf("%s ต้องได้ ErrDatabaseImmutable ได้ %v", name, err)
		}
	}
	low := same
	low.RAMMB = 128
	if _, err := mgr.Update(ctx, svc.ID, 1, ns.ID, low); !errors.Is(err, ErrDatabaseRAMTooLow) {
		t.Errorf("ลด RAM ต่ำกว่าขั้นต่ำต้องได้ ErrDatabaseRAMTooLow ได้ %v", err)
	}

	// service ธรรมดาเปลี่ยนเป็น database ผ่าน Update ไม่ได้แล้ว
	web, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{Name: "web", Image: "nginx", CPUMilli: 100, RAMMB: 128})
	if err != nil {
		t.Fatalf("สร้าง web ไม่สำเร็จ: %v", err)
	}
	if _, err := mgr.Update(ctx, web.ID, 1, ns.ID, UpdateServiceParams{
		Name: "web", Image: "nginx", CPUMilli: 100, RAMMB: 128, ContainerPort: 8080, Replicas: 1, IsDatabase: true,
	}); !errors.Is(err, ErrDatabaseUseTemplate) {
		t.Errorf("สลับ web เป็น database ต้องได้ ErrDatabaseUseTemplate ได้ %v", err)
	}
	if _, err := mgr.Connection(ctx, web.ID, ns.ID); !errors.Is(err, ErrNotTemplateDatabase) {
		t.Errorf("connection ของ web ต้องได้ ErrNotTemplateDatabase ได้ %v", err)
	}
}

// TestCreateWebStorageRejectsBadRequests — web ที่ขอดิสก์ต้องผ่านกติกาจุด mount/replica
func TestCreateWebStorageRejectsBadRequests(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "web-reject-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	cases := []struct {
		name   string
		params CreateServiceParams
		want   error
	}{
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

// TestScaleWithStorageIsRejected — หน้าเว็บซ่อน dropdown ให้แล้ว แต่เส้น PATCH เรียกตรงได้
// ด่านจริงต้องอยู่ที่ service layer ไม่ใช่ที่ UI — กันทุก service ที่มีดิสก์ ไม่ใช่แค่ database
func TestScaleWithStorageIsRejected(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "db-scale-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	pg, err := mgr.CreateDatabase(ctx, 1, ns.ID, CreateDatabaseParams{
		Engine: EnginePostgreSQL, Name: "pg-scale", Username: "appuser", Password: "secret123", Database: "appdb",
		CPUMilli: 300, RAMMB: 256,
	})
	if err != nil {
		t.Fatalf("สร้าง pg-scale ไม่สำเร็จ: %v", err)
	}
	nc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{Name: "nc-scale", Image: "nextcloud:apache",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 80, StorageMB: 2048, DataPath: "/var/www/html"})
	if err != nil {
		t.Fatalf("สร้าง nc-scale ไม่สำเร็จ: %v", err)
	}
	for _, svc := range []*entity.Service{pg, nc} {
		if _, err := mgr.Scale(ctx, svc.ID, ns.ID, 3); !errors.Is(err, ErrStorageReplicas) {
			t.Errorf("scale %s ต้องถูกปฏิเสธด้วย ErrStorageReplicas ได้ %v", svc.Name, err)
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

	pg, err := mgr.CreateDatabase(ctx, 1, ns.ID, CreateDatabaseParams{
		Engine: EnginePostgreSQL, Name: "pg-del", Username: "appuser", Password: "secret123", Database: "appdb",
		CPUMilli: 300, RAMMB: 256, StorageMB: 2048,
	})
	if err != nil {
		t.Fatalf("สร้าง pg-del ไม่สำเร็จ: %v", err)
	}
	nc, err := mgr.Create(ctx, 1, ns.ID, CreateServiceParams{Name: "nc-del", Image: "nextcloud:apache",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 80, StorageMB: 3072, DataPath: "/var/www/html"})
	if err != nil {
		t.Fatalf("สร้าง nc-del ไม่สำเร็จ: %v", err)
	}
	created := []*entity.Service{pg, nc}

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
