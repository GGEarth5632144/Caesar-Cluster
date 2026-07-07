package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/handlers"
	"backend/internal/middleware"
	"backend/internal/repositories"
	"backend/internal/response"
	"backend/internal/services"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DBUrl)
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	defer db.Close()
	log.Println("database connected ✓")

	vmRepo := repositories.NewVMRepo(db)
	allocSvc := services.NewAllocationService(db)

	var prov services.Provisioner
	if cfg.Provisioner == "proxmox" {
		prov = services.NewProxmoxProvisioner(cfg.ProxmoxURL, cfg.ProxmoxToken)
		log.Println("provisioner: PROXMOX")
	} else {
		prov = services.NewMockProvisioner()
		log.Println("provisioner: MOCK")
	}
	vmSvc := services.NewVMService(db, vmRepo, allocSvc, prov)

	vmHandler := handlers.NewVMHandler(vmSvc)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{cfg.FrontendOrigin},
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			c.JSON(503, gin.H{"status": "db down"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.POST("/register", handlers.Register(db))
		api.POST("/login", handlers.Login(db, cfg.JWTSecret))

		protected := api.Group("", middleware.Auth(cfg.JWTSecret))
		{
			protected.GET("/me", func(c *gin.Context) {
				response.OK(c, 200, gin.H{
					"userID": c.GetInt("userID"), "role": c.GetString("role")})
			})
			protected.GET("/vms", vmHandler.List)
			protected.POST("/vms", vmHandler.Create)
			protected.DELETE("/vms/:id", vmHandler.Delete)
		}

		admin := api.Group("/admin", middleware.Auth(cfg.JWTSecret), middleware.AdminOnly())
		{
			admin.GET("/ping", func(c *gin.Context) {
				response.OK(c, 200, "admin ok")
			})
			admin.GET("/nodes", handlers.ListNodes(db))
			admin.POST("/nodes", handlers.CreateNode(db))
		}
	}

	log.Println("server running on http://localhost:" + cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
