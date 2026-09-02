package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// ชื่อ provisioner ที่รองรับ (ตั้งผ่าน env PROVISIONER)
const (
	ProvisionerMock       = "mock"
	ProvisionerKubernetes = "kubernetes"
)

// โหมดการรัน (ตั้งผ่าน env APP_ENV) — แยกออกจาก PROVISIONER โดยตั้งใจ
//
// เดิมโค้ดใช้ PROVISIONER=mock เป็นตัวบอกว่า "อยู่ในโหมด dev" แล้วไปเปิด CORS ให้ localhost ทุกพอร์ต
// ซึ่งมัดสองเรื่องที่ไม่เกี่ยวกันเข้าด้วยกัน: จะแตะคลัสเตอร์จริงหรือไม่ กับจะผ่อน CORS หรือไม่
// พอเอาขึ้นเครื่องจริงแล้วอยากรัน mock ไว้ก่อน (เช่น คลัสเตอร์ยังไม่พร้อม) จะได้ CORS หลวมติดมาด้วย
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Config = ค่า runtime ทั้งหมดที่ระบบต้องใช้ อ่านมาจาก env ครั้งเดียวตอน start
type Config struct {
	AppEnv         string // development | production — คุมความเข้มของ config check
	Port           string
	DBUrl          string
	JWTSecret      string
	FrontendOrigin string
	Provisioner    string // mock | kubernetes
	KubeConfig     string // path ไปยังไฟล์ kubeconfig (ว่าง = ใช้ in-cluster config ตอนรันใน k8s)

	JWTTTLHours        int // อายุ JWT ปกติ (ชม.) — ไม่ติ๊ก remember ตอน login
	JWTRememberTTLDays int // อายุ JWT ตอนติ๊ก "Remember For 30 Days" (วัน)

	// ค่าสำหรับส่งอีเมลรีเซ็ตรหัสผ่านผ่าน SMTP ของบัญชี Gmail แบบ no-reply ที่ทำไว้ให้ระบบนี้
	// (เลิกใช้ Resend API key แล้ว — ไม่ต้องมี key ของบริการภายนอกให้ดูแล/หมดอายุ)
	SMTPHost             string // เซิร์ฟเวอร์ SMTP — Gmail คือ smtp.gmail.com
	SMTPPort             int    // 587 = STARTTLS (ค่าปกติ), 465 = TLS ตั้งแต่ต้น
	SMTPUsername         string // อีเมลเต็มของบัญชี no-reply — ว่าง = ส่งอีเมลไม่ได้ (แค่ warn ไม่ fatal)
	SMTPPassword         string // App Password 16 ตัวของบัญชีนั้น (ไม่ใช่รหัสผ่านที่ใช้ล็อกอิน Google)
	MailFromName         string // ชื่อที่แสดงหน้าอีเมลผู้ส่ง — ตัวที่อยู่จะเป็น SMTPUsername เสมอ
	ResetTokenTTLMinutes int    // อายุของลิงก์รีเซ็ตรหัสผ่าน (นาที)

	// AlertScan = ค่าของตัวสแกน log หา error แล้วส่งเข้าหน้า Alerts
	AlertScan AlertScanConfig
}

// AlertScanConfig = ค่าที่คุม LogAlertScanner (ตัวที่เดินอ่าน log ของทุก service เป็นรอบๆ)
type AlertScanConfig struct {
	// Enabled = เปิด/ปิดตัวสแกนทั้งก้อน — ปิดแล้วหน้า Alerts ยังใช้งานได้ปกติ
	// เพียงแต่จะไม่มีแจ้งเตือนใหม่จาก log เข้ามาเอง
	Enabled bool

	// IntervalSeconds = ทุกกี่วินาทีจะเดินสแกนหนึ่งรอบ
	// ค่า default 60 วินาที: ถี่พอที่ผู้ใช้จะรู้ตัวว่า service พังภายในหนึ่งนาที แต่ไม่ถี่จน
	// ยิง Kubernetes API รัวๆ บน control-plane ตัวเดียวที่เป็น Atom
	IntervalSeconds int

	// MaxLinesPerScan = ดึง log ย้อนหลังสูงสุดกี่บรรทัดต่อ 1 service ต่อ 1 รอบ
	// กัน container ที่พ่น log เป็นหมื่นบรรทัดต่อนาทีดูดแรม/แบนด์วิดท์ของ backend ไปทั้งหมด
	MaxLinesPerScan int

	// IncludeWarnings = ส่ง warning เข้าหน้า Alerts ด้วยหรือไม่
	// default false ตามที่ตกลงกันว่า "ส่งเฉพาะที่มัน error" — เปิดได้ถ้าอยากเห็นละเอียดขึ้น
	IncludeWarnings bool
}

// Load อ่านค่า config จาก environment (โหลด .env ให้ก่อนถ้ามี)
// data flow: ไฟล์ .env / env ของเครื่อง → getEnv ทีละ key → คืน *Config ให้ main ใช้ต่อ
// ถ้าค่าจำเป็น (DB_URL, JWT_SECRET) ขาด จะ log.Fatal หยุดตั้งแต่ต้น (fail fast)
func Load() *Config {
	_ = godotenv.Load() // ไม่มีไฟล์ .env ก็ไม่ error — ใช้ env จริงของเครื่องแทน

	cfg := &Config{
		AppEnv:         normalizeAppEnv(getEnv("APP_ENV", EnvDevelopment)),
		Port:           getEnv("PORT", "8080"),
		DBUrl:          getEnv("DB_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		Provisioner:    getEnv("PROVISIONER", ProvisionerMock),
		KubeConfig:     getEnv("KUBECONFIG", ""),

		JWTTTLHours:        getEnvInt("JWT_TTL_HOURS", 24),
		JWTRememberTTLDays: getEnvInt("JWT_REMEMBER_TTL_DAYS", 30),

		SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:     getEnvInt("SMTP_PORT", 587),
		SMTPUsername: strings.TrimSpace(getEnv("SMTP_USERNAME", "")),
		// Google แสดง App Password เป็น 4 ก้อนคั่นเว้นวรรค ("abcd efgh ijkl mnop") และคนมักคัดลอกมาทั้งอย่างนั้น
		// SMTP ไม่รับช่องว่าง เลยถอดออกให้ตรงนี้ จะได้ไม่ต้องมานั่งงงว่าทำไม auth ไม่ผ่านทั้งที่รหัสถูก
		SMTPPassword:         strings.ReplaceAll(getEnv("SMTP_PASSWORD", ""), " ", ""),
		MailFromName:         getEnv("MAIL_FROM_NAME", "Caesar Cluster"),
		ResetTokenTTLMinutes: getEnvInt("RESET_TOKEN_TTL_MINUTES", 30),

		AlertScan: AlertScanConfig{
			Enabled:         getEnvBool("ALERT_SCAN_ENABLED", true),
			IntervalSeconds: getEnvInt("ALERT_SCAN_INTERVAL_SECONDS", 60),
			MaxLinesPerScan: getEnvInt("ALERT_SCAN_MAX_LINES", 500),
			IncludeWarnings: getEnvBool("ALERT_SCAN_INCLUDE_WARNINGS", false),
		},
	}
	if cfg.DBUrl == "" || cfg.JWTSecret == "" {
		log.Fatal("ต้องกำหนด DB_URL และ JWT_SECRET ใน .env")
	}
	// กันพลาดแบบที่เสียหายที่สุด: เอาขึ้นเครื่องจริงโดยลืมเปลี่ยน secret ตัวอย่าง
	// ใครก็ตามที่อ่าน repo นี้จะปลอม JWT เป็น admin ได้ทันที เลยไม่ยอมให้ start
	if cfg.IsProduction() && (cfg.JWTSecret == "dev-secret" || len(cfg.JWTSecret) < 32) {
		log.Fatal("APP_ENV=production ต้องตั้ง JWT_SECRET ใหม่ยาวอย่างน้อย 32 ตัวอักษร " +
			"สร้างได้ด้วยคำสั่ง: openssl rand -base64 48")
	}
	// อีเมลไม่ใช่ค่าที่ทั้งระบบต้องมีถึงจะ start ได้ (ต่างจาก DB/JWT) — แค่เตือนถ้าลืมตั้ง
	// เพราะจะกระทบเฉพาะฟีเจอร์รีเซ็ตรหัสผ่าน ไม่ควรบล็อกทั้ง server สำหรับคนที่ dev ส่วนอื่นอยู่
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		log.Println("คำเตือน: ไม่ได้ตั้ง SMTP_USERNAME / SMTP_PASSWORD — ระบบส่งอีเมลรีเซ็ตรหัสผ่านจะยังใช้งานไม่ได้")
	}
	return cfg
}

// IsProduction บอกว่ากำลังรันในโหมดเครื่องจริงหรือไม่ — ใช้คุมความเข้มของ config check
func (c *Config) IsProduction() bool { return c.AppEnv == EnvProduction }

// normalizeAppEnv ยอมรับทั้ง prod/production และ dev/development ให้เขียนสั้นได้
func normalizeAppEnv(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "prod", "production":
		return EnvProduction
	default:
		return EnvDevelopment
	}
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

// getEnvBool อ่าน env ที่เป็นค่าเปิด/ปิด — รับได้ทั้ง true/false, 1/0, yes/no, on/off
// ค่าที่แปลไม่ออกจะคืน fallback พร้อม log เตือน (แบบเดียวกับ getEnvInt) ไม่เงียบหายไปเฉยๆ
func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Printf("ค่า env %s=%q ไม่ใช่ค่าเปิด/ปิด ใช้ค่า default %v แทน", key, v, fallback)
		return fallback
	}
}
