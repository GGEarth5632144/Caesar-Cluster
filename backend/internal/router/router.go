package router

import (
	"context"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/config"
	"backend/internal/controller"
	"backend/internal/middlewares"
	"backend/internal/services"
)

// Setup ประกอบ gin.Engine ทั้งหมด: สร้าง controller, ตั้ง CORS, ผูก route → handler
//
// data flow: รับ cfg/db/service layer จาก main → แจกจ่ายให้ controller แต่ละตัว → คืน engine ที่พร้อม r.Run
//
// โครง route:
//
//	public       : GET /health, POST /api/register, POST /api/login
//	ต้อง login   : GET /api/me, GET /api/request-templates,
//	               POST /api/namespaces, POST /api/namespaces/join, GET /api/namespaces/me,
//	               GET|POST /api/services, DELETE /api/services/:id
//	admin only   : POST /api/admin/eligible-students, POST /api/admin/request-templates,
//	               GET /api/admin/namespaces, PATCH /api/admin/namespaces/:id/quota
//
// ลำดับที่ผู้ใช้ต้องเดิน: register → login → สร้าง/เข้าร่วม namespace → deploy service
func Setup(
	cfg *config.Config,
	db *gorm.DB,
	nsMgr *services.NamespaceManager,
	svcMgr *services.ServiceManager,
) *gin.Engine {

	authCtl := controller.NewAuthController(db, cfg.JWTSecret)
	nsCtl := controller.NewNamespaceController(db, nsMgr)
	svcCtl := controller.NewServiceController(db, svcMgr)
	tmplCtl := controller.NewRequestTemplateController(db)
	adminCtl := controller.NewAdminController(db, nsMgr)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{cfg.FrontendOrigin},
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	// /health = liveness/readiness probe: ping DB ภายใน 2 วิ → 200 ถ้าต่อ DB ได้, 503 ถ้าไม่ได้
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			c.JSON(503, gin.H{"status": "db down"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.POST("/register", authCtl.Register)
		api.POST("/login", authCtl.Login)

		protected := api.Group("", middlewares.Auth(cfg.JWTSecret))
		{
			protected.GET("/me", authCtl.Me)
			protected.GET("/request-templates", tmplCtl.List)

			protected.POST("/namespaces", nsCtl.Create)
			protected.POST("/namespaces/join", nsCtl.Join)
			protected.GET("/namespaces/me", nsCtl.Mine)

			protected.GET("/services", svcCtl.List)
			protected.POST("/services", svcCtl.Create)
			protected.DELETE("/services/:id", svcCtl.Delete)
		}

		admin := api.Group("/admin", middlewares.Auth(cfg.JWTSecret), middlewares.AdminOnly())
		{
			admin.POST("/eligible-students", adminCtl.AddEligibleStudents)

            admin.POST("/request-templates", adminCtl.CreateRequestTemplate)
            admin.PATCH("/request-templates/:id", adminCtl.UpdateRequestTemplate)
            admin.DELETE("/request-templates/:id", adminCtl.DeleteRequestTemplate)
			admin.GET("/request-templates", adminCtl.ListAllRequestTemplates)

			admin.GET("/namespaces", adminCtl.ListNamespaces)
			admin.PATCH("/namespaces/:id/quota", adminCtl.SetNamespaceQuota)
		}
	}

	return r
}
