package main

import (
	"log"

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
	//   quota  = คุมโควตาของ namespace (มาแทน AllocationService ที่เคยไล่หา node)
	//   nsMgr  = สร้าง/เข้าร่วม space + ปรับโควตา
	//   svcMgr = deploy/ลบ workload โดยผ่านการเช็คโควตาจาก quota เสมอ
	quota := services.NewQuotaService(db)
	nsMgr := services.NewNamespaceManager(db, quota, prov)
	svcMgr := services.NewServiceManager(db, quota, prov)

	r := router.Setup(cfg, db, nsMgr, svcMgr)

	log.Println("server running on http://localhost:" + cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
