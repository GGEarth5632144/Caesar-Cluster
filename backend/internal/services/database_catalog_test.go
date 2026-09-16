package services

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"backend/internal/entity"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestDatabaseTemplatesFilterEOL — version ที่เลยวัน EOL ต้องหายจากตัวเลือกเอง ไม่ต้องมีใครมาแก้โค้ด
func TestDatabaseTemplatesFilterEOL(t *testing.T) {
	versions := func(now time.Time, engine string) []string {
		for _, e := range DatabaseTemplates(now) {
			if e.Engine == engine {
				out := []string{}
				for _, v := range e.Versions {
					out = append(out, v.Version)
				}
				return out
			}
		}
		return nil
	}

	// PostgreSQL 14 EOL 2026-11-12: ยังเลือกได้ทั้งวันนั้น หายวันถัดไป
	if got := versions(mustDate(t, "2026-11-12"), EnginePostgreSQL); len(got) != 5 {
		t.Errorf("วัน EOL ยังต้องมี PG 14 ได้ %v", got)
	}
	if got := versions(mustDate(t, "2026-11-13"), EnginePostgreSQL); len(got) != 4 || got[len(got)-1] != "15" {
		t.Errorf("หลัง EOL ต้องไม่มี PG 14 ได้ %v", got)
	}

	// default หมดอายุ → ตัวบนสุดที่เหลือเป็น default แทน ไม่ปล่อยให้ไม่มี default
	later := mustDate(t, "2031-01-01")
	for _, e := range DatabaseTemplates(later) {
		n := 0
		for _, v := range e.Versions {
			if v.Default {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s ต้องมี default 1 ตัว ได้ %d", e.Engine, n)
		}
	}
	// ทุก version หมดอายุ → engine หายไปทั้งตัว
	if got := DatabaseTemplates(mustDate(t, "2040-01-01")); len(got) != 0 {
		t.Errorf("ทุก version หมดอายุต้องไม่เหลือ engine ได้ %d", len(got))
	}

	// catalog ต้นฉบับห้ามถูกแก้ตามการกรอง
	if _, _, err := resolveDatabaseVersion(EnginePostgreSQL, "14", mustDate(t, "2026-09-17")); err != nil {
		t.Errorf("การกรองรอบก่อนต้องไม่ลบ PG 14 ออกจาก catalog จริง: %v", err)
	}
	if _, _, err := resolveDatabaseVersion(EnginePostgreSQL, "14", mustDate(t, "2026-12-01")); !errors.Is(err, ErrDatabaseVersionNotFound) {
		t.Errorf("deploy PG 14 หลัง EOL ต้องไม่ได้ ได้ %v", err)
	}
}

// TestBuildConnectionURL — รหัสที่มี @ : / # ต้องถูก encode ไม่งั้น driver ตัด host ผิดที่
// (URL ที่คาดหวังตรงกับที่ทดสอบต่อจริงด้วย node-postgres/mysql2 บนคลัสเตอร์ 2026-09-17)
func TestBuildConnectionURL(t *testing.T) {
	cred := DatabaseCredentials{Username: "appuser", Password: "p@ss:w/rd#1", Database: "appdb"}
	cases := map[string]string{
		EnginePostgreSQL: "postgresql://appuser:p%40ss%3Aw%2Frd%231@mydb:5432/appdb?sslmode=disable",
		EngineMySQL:      "mysql://appuser:p%40ss%3Aw%2Frd%231@mydb:3306/appdb?ssl=false",
		EngineMariaDB:    "mysql://appuser:p%40ss%3Aw%2Frd%231@mydb:3306/appdb?ssl=false",
	}
	for engine, want := range cases {
		e, _ := findDatabaseEngine(engine)
		if got := buildConnectionURL(e, "mydb", cred); got != want {
			t.Errorf("%s: ได้ %q ต้องเป็น %q", engine, got, want)
		}
	}
}

// TestValidateDatabaseCredentials — ค่าที่ต้องผ่าน (ไม่กันเกินจำเป็น)
func TestValidateDatabaseCredentials(t *testing.T) {
	for _, c := range [][4]string{
		{EnginePostgreSQL, "app_user", "p@ss:w/rd#1", "AppDB"},
		{EngineMySQL, "_svc", "Aa1!$%^&*()-_=+[]{};:,.<>/?|~", "shop_2024"},
		{EngineMariaDB, "pg_ok_on_mariadb", "12345678", "_db"},
	} {
		if msg := validateDatabaseCredentials(c[0], c[1], c[2], c[3]); msg != "" {
			t.Errorf("%v ต้องผ่าน แต่ได้ %q", c, msg)
		}
	}
}

func testTemplateDatabaseSvc() *entity.Service {
	return &entity.Service{
		ID: 9, Name: "shop-db", Image: "mysql:8.4",
		CPUMilli: 500, RAMMB: 768, ContainerPort: 3306, Replicas: 1,
		IsDatabase: true, StorageMB: 2048, DataPath: "/var/lib/mysql",
		DatabaseEngine: EngineMySQL, DatabaseVersion: "8.4",
		DBUsername: "appuser", DBName: "appdb", DBPassword: "secret123", DBRootPassword: "rootrandom",
	}
}

// TestK8sDeployTemplateDatabaseUsesSecret — รหัสต้องอยู่ใน Secret และ pod อ้างผ่าน secretKeyRef เท่านั้น
func TestK8sDeployTemplateDatabaseUsesSecret(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	svc := testTemplateDatabaseSvc()

	// Secret ต้องเกิดก่อน StatefulSet — กลับลำดับแล้ว pod ค้าง CreateContainerConfigError
	secretFirst := false
	cs.PrependReactor("create", "statefulsets", func(k8stesting.Action) (bool, runtime.Object, error) {
		_, err := cs.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), "ns-7", "shop-db-credentials")
		secretFirst = err == nil
		return false, nil, nil
	})

	if err := k.DeployService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if !secretFirst {
		t.Error("Secret ต้องถูกสร้างก่อน StatefulSet")
	}

	sec, err := cs.CoreV1().Secrets("ns-7").Get(ctx, "shop-db-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret ต้องถูกสร้าง: %v", err)
	}
	if string(sec.Data[SecretKeyPassword]) != "secret123" || string(sec.Data[SecretKeyRootPassword]) != "rootrandom" {
		t.Errorf("ค่าใน Secret ไม่ตรง: %v", sec.Data)
	}

	sts, err := cs.AppsV1().StatefulSets("ns-7").Get(ctx, "shop-db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("StatefulSet ต้องถูกสร้าง: %v", err)
	}
	env := map[string]corev1.EnvVar{}
	for _, e := range sts.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e
	}
	for name, key := range map[string]string{
		"MYSQL_USER": SecretKeyUsername, "MYSQL_PASSWORD": SecretKeyPassword,
		"MYSQL_DATABASE": SecretKeyDatabase, "MYSQL_ROOT_PASSWORD": SecretKeyRootPassword,
	} {
		e, ok := env[name]
		switch {
		case !ok:
			t.Errorf("ขาด env %s", name)
		case e.Value != "" || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil:
			t.Errorf("env %s ต้องอ่านจาก Secret ไม่ใช่ค่าตรงใน pod spec", name)
		case e.ValueFrom.SecretKeyRef.Name != "shop-db-credentials" || e.ValueFrom.SecretKeyRef.Key != key:
			t.Errorf("env %s ชี้ผิด: %+v", name, e.ValueFrom.SecretKeyRef)
		}
	}
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่ได้ NodePort แต่ได้ %d", *svc.NodePort)
	}

	cred, err := k.DatabaseCredentials(ctx, "ns-7", svc)
	if err != nil || cred.Password != "secret123" || cred.Username != "appuser" || cred.Database != "appdb" {
		t.Errorf("อ่าน credential กลับไม่ตรง: %+v err=%v", cred, err)
	}

	if err := k.DeleteService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	if _, err := cs.CoreV1().Secrets("ns-7").Get(ctx, "shop-db-credentials", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("ลบ database แล้ว Secret ต้องหายด้วย")
	}
}

// TestK8sDeployTemplateDatabaseRollsBackSecret — สร้าง StatefulSet ไม่ผ่านต้องไม่ทิ้งรหัสค้างบนคลัสเตอร์
// และ Secret ที่ค้างจากรอบก่อนต้องถูกเขียนทับ ไม่ทำให้ deploy ล้ม
func TestK8sDeployTemplateDatabaseRollsBackSecret(t *testing.T) {
	stale := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "shop-db-credentials", Namespace: "ns-7"},
		Data: map[string][]byte{SecretKeyPassword: []byte("old")}}
	k, cs := newFakeProvisioner(stale)
	ctx := context.Background()
	cs.PrependReactor("create", "statefulsets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("admission ปฏิเสธ")
	})

	if err := k.DeployService(ctx, "ns-7", testTemplateDatabaseSvc()); err == nil {
		t.Fatal("ต้องคืน error เมื่อสร้าง StatefulSet ไม่ได้")
	}
	if _, err := cs.CoreV1().Secrets("ns-7").Get(ctx, "shop-db-credentials", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("Secret ต้องถูกถอนออกเมื่อ deploy ล้ม")
	}
}
