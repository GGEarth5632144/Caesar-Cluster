package config

import (
	"log"
	"os"
	"strconv"

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

	JWTTTLHours        int // อายุ JWT ปกติ (ชม.) — ไม่ติ๊ก remember ตอน login
	JWTRememberTTLDays int // อายุ JWT ตอนติ๊ก "Remember For 30 Days" (วัน)

	// ค่าสำหรับส่งอีเมลรีเซ็ตรหัสผ่านผ่าน Resend (https://resend.com)
	ResendAPIKey         string // API key ของ Resend — ว่าง = ส่งอีเมลไม่ได้ (แค่ warn ไม่ fatal)
	MailFrom             string // ผู้ส่ง เช่น "Caesar Cluster <no-reply@your-domain>"
	ResetTokenTTLMinutes int    // อายุของลิงก์รีเซ็ตรหัสผ่าน (นาที)
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

		JWTTTLHours:        getEnvInt("JWT_TTL_HOURS", 24),
		JWTRememberTTLDays: getEnvInt("JWT_REMEMBER_TTL_DAYS", 30),

		ResendAPIKey:         getEnv("RESEND_API_KEY", ""),
		MailFrom:             getEnv("MAIL_FROM", "Caesar Cluster <onboarding@resend.dev>"),
		ResetTokenTTLMinutes: getEnvInt("RESET_TOKEN_TTL_MINUTES", 30),
	}
	if cfg.DBUrl == "" || cfg.JWTSecret == "" {
		log.Fatal("ต้องกำหนด DB_URL และ JWT_SECRET ใน .env")
	}
	// อีเมลไม่ใช่ค่าที่ทั้งระบบต้องมีถึงจะ start ได้ (ต่างจาก DB/JWT) — แค่เตือนถ้าลืมตั้ง
	// เพราะจะกระทบเฉพาะฟีเจอร์รีเซ็ตรหัสผ่าน ไม่ควรบล็อกทั้ง server สำหรับคนที่ dev ส่วนอื่นอยู่
	if cfg.ResendAPIKey == "" {
		log.Println("คำเตือน: ไม่ได้ตั้ง RESEND_API_KEY — ระบบส่งอีเมลรีเซ็ตรหัสผ่านจะยังใช้งานไม่ได้")
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

// getEnvInt อ่าน env ที่คาดว่าเป็นตัวเลข — ว่าง/พังจะคืน fallback (พร้อม log เตือนถ้าพัง)
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("ค่า env %s=%q ไม่ใช่ตัวเลข ใช้ค่า default %d แทน", key, v, fallback)
		return fallback
	}
	return n
}
