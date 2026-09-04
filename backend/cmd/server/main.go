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
	"backend/internal/controller"
	"backend/internal/router"
	"backend/internal/services"
)

// main = จุดเริ่มของ API server: อ่าน config → ต่อ DB + AutoMigrate → เลือก provisioner
// → ประกอบ service layer → router.Setup → เปิด port รอ request
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl) // connect + AutoMigrate ในตัว

	// ทดสอบค่า SMTP ตั้งแต่ start แทนที่จะไปรู้ตอนคนแรกกดสมัครแล้วไม่ผ่าน (ส่งไม่ออก = สมัครไม่ได้เลย)
	// ไม่ log.Fatal เพราะ SMTP ล่มชั่วคราวไม่ควรทำให้ทั้งระบบขึ้นไม่ได้ ส่วน "ลืมตั้งค่า" config.Load กันไว้แล้ว
	smtpCtx, smtpCancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := controller.CheckMailer(smtpCtx, cfg); err != nil {
		log.Printf("คำเตือน: ต่อ SMTP ไม่สำเร็จ — สมัครสมาชิกและรีเซ็ตรหัสผ่านจะใช้งานไม่ได้จนกว่าจะแก้: %v", err)
	} else {
		log.Println("smtp connection verified ✓")
	}
	smtpCancel()

	// เลือกตัวสร้างของจริงบน cluster: kubernetes ของจริง หรือ mock ตอน dev
	var prov services.Provisioner
	if cfg.Provisioner == config.ProvisionerKubernetes {
		prov = services.NewKubernetesProvisioner(cfg.KubeConfig)
		log.Println("provisioner: KUBERNETES")
	} else {
		prov = services.NewMockProvisioner()
		log.Println("provisioner: MOCK")
	}

	// service layer: quota คุมโควตาของ namespace, nsMgr สร้าง/เข้าร่วม space,
	// svcMgr deploy/ลบ workload (ผ่านการเช็คโควตาเสมอ), inviteMgr คำเชิญเข้ากลุ่ม (ไม่แตะ cluster)
	quota := services.NewQuotaService(db)
	nsMgr := services.NewNamespaceManager(db, quota, prov)
	svcMgr := services.NewServiceManager(db, quota, prov)
	inviteMgr := services.NewInviteManager(db)
	telemetrySvc := services.NewTelemetryService(db)
	telemetrySvc.StartTelemetryWorker()

	alertMgr := services.NewAlertManager(db)

	// ผูก context กับสัญญาณปิดโปรแกรม ให้ HTTP server และ background worker ปิดจากสัญญาณเดียวกัน
	// เดิม r.Run() ไม่ผูกกับ context นี้ กด Ctrl+C แล้วตัวสแกนหยุดแต่โปรแกรมค้างต่อ
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

	// ListenAndServe ล้มเหลวแล้วห้าม log.Fatal ตรงนี้ — os.Exit จะข้าม Shutdown และ wg.Wait()
	// ทำให้ตัวสแกน log ถูกฆ่ากลางรอบ เรียก stop() ให้เดินทางปิดปกติแล้วค่อยจบด้วย exit code ที่ถูก
	var serveErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stop()
		log.Println("server running on http://localhost:" + cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
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
	if serveErr != nil {
		log.Fatalf("HTTP server หยุดทำงานเพราะข้อผิดพลาด: %v", serveErr)
	}
	log.Println("ปิดเซิร์ฟเวอร์เรียบร้อย")
}
