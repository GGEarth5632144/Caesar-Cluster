package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

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

	// gin debug mode พิมพ์ทุก request ออก stdout และโชว์ route ทั้งหมดตอน start
	// บนเครื่องจริงเป็นได้ทั้งขยะใน log และการเปิดเผยโครง API โดยไม่จำเป็น
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	db := config.ConnectDB(cfg.DBUrl) // connect + AutoMigrate ในตัว

	// เลือกตัวสร้างของจริงบน cluster: kubernetes ของจริง หรือ mock ตอน dev
	//
	// ต่อคลัสเตอร์ไม่ได้ = ไม่ยอม start เลย (NewKubernetesProvisioner ทดสอบการเชื่อมต่อให้ตั้งแต่ตอนสร้าง)
	// ตั้งใจให้พังตั้งแต่ต้น ดีกว่าปล่อยให้ server ขึ้นมาแล้วผู้ใช้มาเจอ error ตอนกด deploy
	var prov services.Provisioner
	if cfg.Provisioner == config.ProvisionerKubernetes {
		k8sProv, err := services.NewKubernetesProvisioner(cfg.KubeConfig, cfg.K8s)
		if err != nil {
			log.Fatalf("เตรียม kubernetes provisioner ไม่สำเร็จ: %v", err)
		}
		prov = k8sProv
		log.Println("provisioner: KUBERNETES")
	} else {
		prov = services.NewMockProvisioner()
		log.Println("provisioner: MOCK — ไม่มีอะไรถูกสร้างจริงบนคลัสเตอร์")
	}

	// ประกอบ service layer:
	//   quota     = คุมโควตาของ namespace (มาแทน AllocationService ที่เคยไล่หา node)
	//   nsMgr     = สร้าง/เข้าร่วม space + ปรับโควตา
	//   svcMgr    = deploy/ลบ workload โดยผ่านการเช็คโควตาจาก quota เสมอ
	//   inviteMgr = เชิญ/ตอบรับ/ปฏิเสธคำเชิญเข้ากลุ่ม (ไม่แตะ cluster เลย ไม่ต้องรับ prov)
	quota := services.NewQuotaService(db)
	nsMgr := services.NewNamespaceManager(db, quota, prov)
	svcMgr := services.NewServiceManager(db, quota, prov, cfg.AllowedImageRegistries)
	inviteMgr := services.NewInviteManager(db)
	alertMgr := services.NewAlertManager(db)

	// ตัวสแกน log หา error แล้วสร้างแจ้งเตือน — รันเป็นงานเบื้องหลังคู่ไปกับ HTTP server
	//
	// ผูก context ไว้กับสัญญาณปิดโปรแกรม (SIGINT/SIGTERM) เพื่อให้รอบที่กำลังอ่าน log ค้างอยู่
	// ถูกตัดทันทีตอน container ถูกสั่งหยุด ไม่ค้างรอ Kubernetes API จนโดน SIGKILL
	scanCtx, stopScan := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopScan()
	go services.NewLogAlertScanner(db, prov, alertMgr, services.LogScanConfig{
		Enabled:         cfg.AlertScan.Enabled,
		Interval:        time.Duration(cfg.AlertScan.IntervalSeconds) * time.Second,
		MaxLinesPerScan: int64(cfg.AlertScan.MaxLinesPerScan),
		IncludeWarnings: cfg.AlertScan.IncludeWarnings,
	}).Run(scanCtx)

	r := router.Setup(cfg, db, nsMgr, svcMgr, inviteMgr, alertMgr)

	log.Printf("server running on :%s (env=%s)", cfg.Port, cfg.AppEnv)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
