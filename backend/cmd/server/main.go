package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"backend/internal/config"
	"backend/internal/router"
	"backend/internal/services"
)

// main = จุดเริ่มของ API server — ประกอบทุก dependency เข้าด้วยกันแล้วเปิดรับ HTTP
//
// data flow:
//   - อ่าน config จาก env (config.Load) → ต่อ DB + AutoMigrate (config.ConnectDB)
//   - เลือก provisioner (mock/kubernetes) ตาม env แล้วฉีดเข้า service layer
//   - ประกอบ service layer แล้วส่งให้ router.Setup → r.Run เปิด port รอ request
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl) // connect + AutoMigrate ในตัว

	// เลือกตัวสร้างของจริงบน cluster: kubernetes ของจริง หรือ mock ตอน dev
	var prov services.Provisioner
	if cfg.Provisioner == config.ProvisionerKubernetes {
		prov = services.NewKubernetesProvisioner(cfg.KubeConfig)
		log.Println("provisioner: KUBERNETES")
	} else {
		prov = services.NewMockProvisioner()
		log.Println("provisioner: MOCK")
	}

	// ประกอบ service layer:
	//   quota     = คุมโควตาของ namespace (มาแทน AllocationService ที่เคยไล่หา node)
	//   nsMgr     = สร้าง/เข้าร่วม space + ปรับโควตา
	//   svcMgr    = deploy/ลบ workload โดยผ่านการเช็คโควตาจาก quota เสมอ
	//   inviteMgr = เชิญ/ตอบรับ/ปฏิเสธคำเชิญเข้ากลุ่ม (ไม่แตะ cluster เลย ไม่ต้องรับ prov)
	quota := services.NewQuotaService(db)
	nsMgr := services.NewNamespaceManager(db, quota, prov)
	svcMgr := services.NewServiceManager(db, quota, prov)
	inviteMgr := services.NewInviteManager(db)
	telemetrySvc := services.NewTelemetryService(db)
	telemetrySvc.StartTelemetryWorker()

	alertMgr := services.NewAlertManager(db)

	// ผูก context ไว้กับสัญญาณปิดโปรแกรม (SIGINT/SIGTERM) ให้ทั้ง HTTP server และ background
	// worker ปิดตัวจากสัญญาณเดียวกัน — เดิม r.Run() (ที่ ListenAndServe ข้างในบล็อกอยู่)
	// ไม่ได้ผูกกับ context นี้เลย กด Ctrl+C แล้วตัวสแกน log จะหยุดแต่ตัวโปรแกรมค้างต่อ
	// เพราะ HTTP server ยังรอ connection อยู่ ไม่มีอะไรไปสั่ง Shutdown ให้
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		services.NewLogAlertScanner(db, prov, alertMgr, services.LogScanConfig{
			Enabled:         cfg.AlertScan.Enabled,
			Interval:        time.Duration(cfg.AlertScan.IntervalSeconds) * time.Second,
			MaxLinesPerScan: int64(cfg.AlertScan.MaxLinesPerScan),
			IncludeWarnings: cfg.AlertScan.IncludeWarnings,
		}).Run(ctx)
	}()

	r := router.Setup(cfg, db, nsMgr, svcMgr, inviteMgr, telemetrySvc, alertMgr)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("server running on http://localhost:" + cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("กำลังปิดเซิร์ฟเวอร์...")

	// ให้เวลา request ที่ค้างอยู่ตอนนี้ทำงานจนจบก่อนปิดจริง แทนการตัดทิ้งทันที
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("ปิด HTTP server ไม่ราบรื่น: %v", err)
	}

	wg.Wait()
	log.Println("ปิดเซิร์ฟเวอร์เรียบร้อย")
}
