package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// ชื่อ provisioner ที่รองรับ (ตั้งผ่าน env PROVISIONER)
const (
	ProvisionerMock       = "mock"
	ProvisionerKubernetes = "kubernetes"
)

// Config = ค่า runtime ทั้งหมดที่ระบบต้องใช้ อ่านมาจาก env ครั้งเดียวตอน start
type Config struct {
	Port           string
	DBUrl          string
	JWTSecret      string
	FrontendOrigin string
	Provisioner    string // mock | kubernetes
	KubeConfig     string // path ไปยังไฟล์ kubeconfig (ว่าง = ใช้ in-cluster config ตอนรันใน k8s)
}

// Load อ่านค่า config จาก environment (โหลด .env ให้ก่อนถ้ามี)
// data flow: ไฟล์ .env / env ของเครื่อง → getEnv ทีละ key → คืน *Config ให้ main ใช้ต่อ
// ถ้าค่าจำเป็น (DB_URL, JWT_SECRET) ขาด จะ log.Fatal หยุดตั้งแต่ต้น (fail fast)
func Load() *Config {
	_ = godotenv.Load() // ไม่มีไฟล์ .env ก็ไม่ error — ใช้ env จริงของเครื่องแทน

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		DBUrl:          getEnv("DB_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		Provisioner:    getEnv("PROVISIONER", ProvisionerMock),
		KubeConfig:     getEnv("KUBECONFIG", ""),
	}
	if cfg.DBUrl == "" || cfg.JWTSecret == "" {
		log.Fatal("ต้องกำหนด DB_URL และ JWT_SECRET ใน .env")
	}
	return cfg
}

// getEnv อ่าน env ตาม key — ถ้าไม่มีหรือค่าว่างให้คืน fallback แทน
// data flow: os.Getenv(key) → คืนค่าที่เจอ หรือ fallback ให้ Load นำไปเก็บใน Config
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
