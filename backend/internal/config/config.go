package config

import (
	"log"
	"net"
	"net/url"
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

// โหมดการรัน (env APP_ENV) — แยกจาก PROVISIONER โดยตั้งใจ: รัน mock บนเครื่องจริง
// ไม่ควรลากเอา CORS หลวมติดไปด้วย (ใช้ที่ router.allowOriginFor และ cmd/seed)
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

	// ค่าสำหรับส่งอีเมลผ่าน SMTP ของบัญชี Gmail แบบ no-reply ที่ทำไว้ให้ระบบนี้
	SMTPHost             string // เซิร์ฟเวอร์ SMTP — Gmail คือ smtp.gmail.com
	SMTPPort             int    // 587 = STARTTLS (ค่าปกติ), 465 = TLS ตั้งแต่ต้น
	SMTPUsername         string // อีเมลเต็มของบัญชี no-reply — ว่าง = ส่งอีเมลไม่ได้ (แค่ warn ไม่ fatal)
	SMTPPassword         string // App Password 16 ตัวของบัญชีนั้น (ไม่ใช่รหัสผ่านที่ใช้ล็อกอิน Google)
	MailFromName         string // ชื่อที่แสดงหน้าอีเมลผู้ส่ง — ตัวที่อยู่จะเป็น SMTPUsername เสมอ
	ResetTokenTTLMinutes int    // อายุของลิงก์รีเซ็ตรหัสผ่าน (นาที)
	// อายุลิงก์ยืนยันอีเมล (ชม.) — ยาวกว่าลิงก์รีเซ็ตมาก เพราะคนเพิ่งสมัครอาจเปิดเมลวันรุ่งขึ้น
	VerifyTokenTTLHours int
	// อีเมลที่ผู้ใช้ตอบกลับได้จริง (ว่าง = ไม่ใส่ Reply-To) — no-reply ล้วนโดนตัวกรองสแปมหักคะแนน
	MailReplyTo string

	// เก็บ power_histories ย้อนหลังกี่วัน (0 หรือติดลบ = ไม่ลบเลย)
	// ห้ามต่ำกว่า 30 เพราะกราฟเลือกดูย้อนหลังได้ไกลสุด 30 วัน
	PowerHistoryRetentionDays int
}

// Load อ่าน config จาก environment (โหลด .env ให้ก่อนถ้ามี)
// ค่าจำเป็น (DB_URL, JWT_SECRET) ขาด → log.Fatal หยุดตั้งแต่ต้น
func Load() *Config {
	// .env อยู่ที่ root ของ repo — `go run ./cmd/server` จาก backend/ จึงต้องเดินขึ้นไปหยิบที่ ../.env
	// (godotenv ไม่เขียนทับค่าที่มีอยู่แล้ว env ของเครื่องจริงจึงชนะเสมอ)
	_ = godotenv.Load() // ไม่มีไฟล์ .env ก็ไม่ error
	_ = godotenv.Load("../.env")

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
		// Google แสดง App Password เป็น 4 ก้อนคั่นเว้นวรรค แต่ SMTP ไม่รับช่องว่าง — ถอดออกให้ตรงนี้
		SMTPPassword:         strings.ReplaceAll(getEnv("SMTP_PASSWORD", ""), " ", ""),
		MailFromName:         getEnv("MAIL_FROM_NAME", "Caesar Cluster"),
		ResetTokenTTLMinutes: getEnvInt("RESET_TOKEN_TTL_MINUTES", 30),
		VerifyTokenTTLHours:  getEnvInt("VERIFY_TOKEN_TTL_HOURS", 24),
		MailReplyTo:          strings.TrimSpace(getEnv("MAIL_REPLY_TO", "")),

		PowerHistoryRetentionDays: getEnvInt("POWER_HISTORY_RETENTION_DAYS", 30),
	}
	if cfg.DBUrl == "" || cfg.JWTSecret == "" {
		log.Fatal("ต้องกำหนด DB_URL และ JWT_SECRET ใน .env")
	}
	// ลืมเปลี่ยน secret ตัวอย่างบนเครื่องจริง = ใครอ่าน repo นี้ก็ปลอม JWT เป็น admin ได้
	// (ค่าตัวอย่าง "dev-secret" ตกด่านความยาวอยู่แล้ว ไม่ต้องเช็คแยก)
	if cfg.IsProduction() && len(cfg.JWTSecret) < 32 {
		log.Fatal("APP_ENV=production ต้องตั้ง JWT_SECRET ใหม่ยาวอย่างน้อย 32 ตัวอักษร " +
			"สร้างได้ด้วยคำสั่ง: openssl rand -base64 48")
	}
	// ส่งอีเมลไม่ออก = สมัครสมาชิกไม่ได้เลย — เครื่องจริงจึงไม่ยอม start
	// ตอน dev เตือนเฉยๆ (คนทำฟีเจอร์อื่นไม่ควรต้องมี App Password ก่อน)
	if cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		if cfg.IsProduction() {
			log.Fatal("APP_ENV=production ต้องตั้ง SMTP_USERNAME และ SMTP_PASSWORD " +
				"ไม่งั้นระบบสมัครสมาชิก (ลิงก์ยืนยันอีเมล) และรีเซ็ตรหัสผ่านใช้งานไม่ได้ทั้งคู่")
		}
		log.Println("คำเตือน: ไม่ได้ตั้ง SMTP_USERNAME / SMTP_PASSWORD — " +
			"สมัครสมาชิกและรีเซ็ตรหัสผ่านจะยังใช้งานไม่ได้ (ส่งอีเมลไม่ออก)")
	}
	// ลิงก์ในอีเมลชี้ไปที่ FRONTEND_ORIGIN — เป็น localhost/IP แปลว่าผู้รับกดไม่ได้ และส่อสแปม
	if cfg.IsProduction() && !hasPublicHost(cfg.FrontendOrigin) {
		log.Printf("คำเตือน: FRONTEND_ORIGIN=%q ไม่ได้ชี้ไปที่โดเมนสาธารณะ — "+
			"ลิงก์ยืนยันอีเมล/รีเซ็ตรหัสผ่านที่ส่งออกไปจะกดไม่ได้ และเสี่ยงถูกจัดเป็นสแปม", cfg.FrontendOrigin)
	}
	return cfg
}

// IsProduction บอกว่ากำลังรันในโหมดเครื่องจริงหรือไม่ — ใช้คุมความเข้มของ config check
func (c *Config) IsProduction() bool { return c.AppEnv == EnvProduction }

// normalizeAppEnv ยอมรับทั้ง prod/production และ dev/development
// ค่าที่สะกดผิดต้องเตือนดังๆ — APP_ENV=prodution พลาดตัวเดียวคือปิดด่านกัน JWT_SECRET อ่อนทั้งด่าน
func normalizeAppEnv(v string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(v)); normalized {
	case "prod", "production":
		return EnvProduction
	case "", "dev", "development":
		return EnvDevelopment
	default:
		log.Printf("คำเตือน: ค่า APP_ENV=%q ไม่รู้จัก ใช้โหมด %s แทน "+
			"(ถ้าตั้งใจจะรันเครื่องจริงต้องเป็น APP_ENV=production เท่านั้น)", v, EnvDevelopment)
		return EnvDevelopment
	}
}

// getEnv อ่าน env ตาม key — ไม่มีหรือว่างให้คืน fallback
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

// hasPublicHost บอกว่า origin ชี้ไปโดเมนจริงไหม — ใช้ตัดสินแค่ว่าจะเตือนหรือไม่ เกณฑ์จึงหยาบได้:
// อะไรที่ไม่ใช่ loopback และไม่ใช่ IP ล้วน ถือว่าเป็นโดเมน
func hasPublicHost(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return false
	}
	// IP ล้วน (ไม่ว่าจะ public หรือไม่) ใช้เป็นปลายทางของลิงก์ในอีเมลไม่ได้อยู่ดี —
	// ไม่มี TLS cert ที่เบราว์เซอร์เชื่อ และตัวกรองสแปมมองว่าเป็นลิงก์น่าสงสัยเสมอ
	return net.ParseIP(host) == nil
}
