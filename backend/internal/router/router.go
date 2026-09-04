package router

import (
	"context"
	"net/url"
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
// แบ่งเป็น 3 กลุ่ม — public, ต้อง login (middlewares.Auth), admin only (+ AdminOnly)
//
// ลำดับที่ผู้ใช้ต้องเดิน: register → กดลิงก์ยืนยันในอีเมล (ได้ JWT ตรงนั้นเลย ไม่ต้องผ่าน login)
// → สร้าง/เข้าร่วม namespace → deploy service
func Setup(
	cfg *config.Config,
	db *gorm.DB,
	nsMgr *services.NamespaceManager,
	svcMgr *services.ServiceManager,
	inviteMgr *services.InviteManager,
	telemetrySvc *services.TelemetryService,
	alertMgr *services.AlertManager,
) *gin.Engine {
	// โหมด release ตอน production: ปิด route dump ตอน start และ debug log ราย request
	// ที่ไม่ควรอยู่บนเครื่องจริง (ตรงกับที่ backend/.env.example บอกไว้เรื่อง APP_ENV)
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	authCtl := controller.NewAuthController(db, cfg)
	nsCtl := controller.NewNamespaceController(db, nsMgr)
	svcCtl := controller.NewServiceController(db, svcMgr)
	tmplCtl := controller.NewRequestTemplateController(db)
	adminCtl := controller.NewAdminController(db, nsMgr, svcMgr)
	reqCtl := controller.NewRequestController(db)
	aiReviewReqCtl := controller.NewAIReviewRequestController(db)
	inviteCtl := controller.NewInviteController(inviteMgr)
	telemetryCtrl := controller.NewTelemetryController(telemetrySvc)
	alertCtl := controller.NewAlertController(alertMgr)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOriginFunc: allowOriginFor(cfg),
		AllowMethods:    []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Authorization", "Content-Type"},
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

		// ยืนยันอีเมลด้วยลิงก์ที่ส่งตอนสมัคร (public — ยังไม่มี JWT ตอนเรียกสองเส้นนี้)
		//
		// /verify-email ไม่ใส่ rate limit เพราะ token 32 ไบต์เดาไม่ได้อยู่แล้ว มีแต่จะบล็อกนักศึกษา
		// ทั้งแล็บที่ใช้ IP เดียวกัน ส่วน /resend-verification ใส่เพราะสั่งให้ระบบยิงเมลไปหาที่อยู่ที่
		// ผู้เรียกกรอกเอง (คุมต่อบัญชีด้วย cooldown 60 วิ ใน sendVerificationLink อีกชั้น)
		api.POST("/verify-email", authCtl.VerifyEmail)
		api.POST("/resend-verification", middlewares.RateLimit(5, 15*time.Minute), authCtl.ResendVerification)

		// รีเซ็ตรหัสผ่านผ่านอีเมล (public) — /forgot-password มี rate limit ต่อ IP กันสแปม/email-bombing
		api.POST("/forgot-password", middlewares.RateLimit(3, 15*time.Minute), authCtl.ForgotPassword)
		api.POST("/reset-password", authCtl.ResetPassword)
		api.GET("/telemetry", telemetryCtrl.GetTelemetry)
		api.GET("/telemetry/history", telemetryCtrl.GetTelemetryHistory)

		protected := api.Group("", middlewares.Auth(cfg.JWTSecret, db))
		{
			protected.GET("/me", authCtl.Me)
			protected.GET("/request-templates", tmplCtl.List)

			protected.GET("/requests", reqCtl.ListMine)
			protected.POST("/requests", reqCtl.Create)

			protected.POST("/namespaces", nsCtl.Create)
			protected.POST("/namespaces/join", nsCtl.Join)
			protected.GET("/namespaces/me", nsCtl.Mine)
			protected.DELETE("/namespaces", nsCtl.Leave)

			protected.POST("/namespaces/invites", inviteCtl.Create)
			protected.GET("/namespaces/invites/mine", inviteCtl.Mine)
			protected.GET("/namespaces/invites/sent", inviteCtl.Sent)
			protected.PATCH("/namespaces/invites/:id/accept", inviteCtl.Accept)
			protected.PATCH("/namespaces/invites/:id/decline", inviteCtl.Decline)
			protected.DELETE("/namespaces/invites/:id", inviteCtl.Cancel)

			protected.GET("/services", svcCtl.List)
			protected.POST("/services", svcCtl.Create)
			protected.DELETE("/services/:id", svcCtl.Delete)
			protected.GET("/services/:id/logs", svcCtl.Logs)

			// /unread-count แยกเป็น endpoint เบาๆ ที่ตอบแค่ตัวเลข เพราะหน้าเว็บ poll ถี่ที่สุดในกลุ่มนี้
			// DELETE /alerts/read ประกาศก่อน /alerts/:id ให้เห็นชัดว่าตั้งใจ (gin จับ static ก่อน wildcard อยู่แล้ว)
			protected.GET("/alerts", alertCtl.List)
			protected.GET("/alerts/unread-count", alertCtl.UnreadCount)
			protected.PATCH("/alerts/read", alertCtl.MarkRead)
			protected.DELETE("/alerts/read", alertCtl.DeleteRead)
			protected.DELETE("/alerts/:id", alertCtl.Delete)

			// "ใบเสร็จ" ของ deploy request ที่ส่งเข้า Cluster-AI — ให้ AIReviewPage.tsx ดึงกลับมาได้ถ้า
			// router state หาย (refresh/เปิดลิงก์ตรง) เพราะ Cluster-AI เองไม่เก็บ service_name/image/cpu/ram
			protected.POST("/ai-review-requests", aiReviewReqCtl.Create)
			protected.GET("/ai-review-requests/:request_id", aiReviewReqCtl.Get)
		}

		admin := api.Group("/admin", middlewares.Auth(cfg.JWTSecret, db), middlewares.AdminOnly())
		{
			admin.GET("/eligible-students", adminCtl.ListEligibleStudents)
			admin.POST("/eligible-students", adminCtl.AddEligibleStudents)
			admin.POST("/eligible-students/preview", adminCtl.PreviewEligibleStudents)

			admin.POST("/request-templates", adminCtl.CreateRequestTemplate)
			admin.PATCH("/request-templates/:id", adminCtl.UpdateRequestTemplate)
			admin.DELETE("/request-templates/:id", adminCtl.DeleteRequestTemplate)
			admin.GET("/request-templates", adminCtl.ListAllRequestTemplates)

			admin.GET("/namespaces", adminCtl.ListNamespaces)
			admin.PATCH("/namespaces/:id/quota", adminCtl.SetNamespaceQuota)
			admin.DELETE("/namespaces/:id", adminCtl.DeleteNamespace)

			admin.GET("/requests", adminCtl.ListAllRequests)
			admin.PATCH("/requests/:id/approve", adminCtl.Approve)
			admin.PATCH("/requests/:id/deny", adminCtl.Deny)

			admin.GET("/email-deliveries", adminCtl.ListEmailDeliveries)

			admin.GET("/users", adminCtl.ListUsers)
			admin.PATCH("/users/:id", adminCtl.UpdateUser)
			admin.DELETE("/users/:id", adminCtl.DeleteUser)
		}
	}

	return r
}

// allowOriginFor สร้างฟังก์ชันเช็ค CORS origin — FRONTEND_ORIGIN ผ่านเสมอ ส่วน localhost/127.0.0.1
// ทุกพอร์ตผ่านเพิ่มเฉพาะตอน dev (Vite สลับพอร์ตเองได้ และ browser มอง 127.0.0.1 เป็นคนละ origin)
//
// ตัดสินจาก APP_ENV ไม่ใช่ PROVISIONER — รัน mock บนเครื่องจริงไม่ใช่เหตุผลให้เปิด CORS ให้ localhost
func allowOriginFor(cfg *config.Config) func(string) bool {
	devMode := !cfg.IsProduction()
	return func(origin string) bool {
		if origin == cfg.FrontendOrigin {
			return true
		}
		if !devMode {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
}
