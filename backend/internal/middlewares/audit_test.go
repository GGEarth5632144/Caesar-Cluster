package middlewares

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/entity"
)

// ต้องมี Postgres จริง (เหมือน services/service_manager_rollback_test.go) — ไม่มี DB ก็ข้าม
func TestAuditRecordsOnlySuccessfulWrites(t *testing.T) {
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5433/cloud_cluster?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("ข้าม: ต่อ DB ทดสอบไม่ได้ (%v)", err)
	}
	if err := db.AutoMigrate(&entity.AuditLog{}); err != nil {
		t.Skipf("ข้าม: migrate audit_logs ไม่ได้ (%v)", err)
	}

	const actor = "audit-test-actor"
	t.Cleanup(func() { db.Exec("DELETE FROM audit_logs WHERE actor_name = ?", actor) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api", func(c *gin.Context) {
		c.Set(CtxRole, "admin")
		c.Set(CtxRealName, actor)
	}, Audit(db))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	api.POST("/services", ok)
	api.PATCH("/admin/requests/:id/approve", ok)
	api.POST("/services/node-port-reservation", ok)
	api.POST("/requests", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	api.GET("/services", ok)

	for _, req := range []struct{ method, path string }{
		{"POST", "/api/services"},
		{"PATCH", "/api/admin/requests/7/approve"},
		{"POST", "/api/services/node-port-reservation"}, // ข้ามโดยตั้งใจ
		{"POST", "/api/requests"},                       // 400 ไม่บันทึก
		{"GET", "/api/services"},                        // อ่านอย่างเดียวไม่บันทึก
	} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(req.method, req.path, nil))
	}

	var rows []entity.AuditLog
	db.Where("actor_name = ?", actor).Order("id").Find(&rows)
	if len(rows) != 2 {
		t.Fatalf("ต้องได้ 2 แถว ได้ %d: %+v", len(rows), rows)
	}
	if rows[0].EventType != "CREATE" || rows[0].ActionTitle != "สร้าง service" || rows[0].Detail != "POST /api/services" {
		t.Errorf("แถวแรกผิด: %+v", rows[0])
	}
	if rows[1].EventType != "APPROVE" || rows[1].Detail != "PATCH /api/admin/requests/7/approve" {
		t.Errorf("แถวสองผิด: %+v", rows[1])
	}
}
