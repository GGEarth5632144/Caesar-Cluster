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

// ระดับ Pod Security Admission ที่รองรับ (ตั้งผ่าน env K8S_POD_SECURITY)
const (
	PodSecurityBaseline   = "baseline"
	PodSecurityRestricted = "restricted"
)

// Config = ค่า runtime ทั้งหมดที่ระบบต้องใช้ อ่านมาจาก env ครั้งเดียวตอน start
type Config struct {
	AppEnv         string // development | production — คุมความเข้มของ CORS (ดู router.allowOriginFor)
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

	// K8s = ค่าที่ใช้เฉพาะตอน PROVISIONER=kubernetes
	K8s K8sConfig

	// AllowedImageRegistries = prefix ของ image ที่ผู้ใช้ deploy ได้ (คั่นด้วย comma)
	// ว่าง = อนุญาตทุก image (พฤติกรรมเดิม) — ตั้งบนเครื่องจริงเพื่อกันคนเอา image ขุดเหรียญมารัน
	AllowedImageRegistries []string

	// AlertScan = ค่าของตัวสแกน log หา error แล้วส่งเข้าหน้า Alerts
	AlertScan AlertScanConfig
}

// AlertScanConfig = ค่าที่คุม LogAlertScanner (ตัวที่เดินอ่าน log ของทุก service เป็นรอบๆ)
//
// แยกเป็นก้อนของตัวเองเพราะเป็นงานเบื้องหลังที่ปิดทิ้งได้ทั้งก้อนโดยระบบส่วนอื่นไม่กระทบ
// (ต่างจาก K8sConfig ที่ถ้าตั้งผิดคือ deploy ไม่ได้เลย)
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

// K8sConfig = ค่าที่ KubernetesProvisioner ต้องใช้ตอนสร้างของจริงบนคลัสเตอร์
//
// ทั้งหมดเป็นค่าที่ผูกกับ "คลัสเตอร์เครื่องนี้" ไม่ใช่กติกาธุรกิจ เลยแยกเป็น env
// ไม่ฝังเป็น constant ในโค้ด (คลัสเตอร์ที่ pod CIDR ต่างกันจะได้ไม่ต้องแก้โค้ด)
type K8sConfig struct {
	// PodCIDR = ช่วง IP ของ pod ทั้งคลัสเตอร์ (ตามเอกสาร Calico ของคลัสเตอร์นี้คือ 172.16.0.0/16)
	// NetworkPolicy ใช้ค่านี้กันไม่ให้ pod ใน namespace หนึ่งคุยข้ามไปหา pod ของ namespace อื่น
	PodCIDR string

	// BlockedEgressCIDRs = ช่วง IP ที่ห้าม pod ของนักศึกษาต่อออกไป
	// ค่า default คือวง VLAN100 ของ node ทุกตัว — กันไม่ให้ container ที่รันอยู่ยิงเข้า SSH/kubelet
	// ของ node หรือเข้า control plane ตรงๆ (pod ควรออกอินเทอร์เน็ตได้ แต่ไม่ควรเดินในบ้านเรา)
	BlockedEgressCIDRs []string

	// DefaultContainerPort = port ที่ใช้เมื่อ service ไม่ได้ระบุ container_port และไม่มี env PORT
	DefaultContainerPort int

	// PodSecurity = ระดับ Pod Security Admission ที่ enforce บน namespace ของผู้ใช้
	// baseline   = กัน privileged / hostPath / hostNetwork (image ทั่วไปยังรันได้ รวมที่รันเป็น root)
	// restricted = เข้มสุด บังคับ runAsNonRoot ด้วย — image จำนวนมากจะรันไม่ขึ้น ต้องเลือกให้ดี
	PodSecurity string

	// ImagePullPolicy ของ container ที่ deploy (Always | IfNotPresent | Never)
	// IfNotPresent เหมาะกับคลัสเตอร์นี้ที่ node เป็น Atom + SSD 64GB และดึง image ผ่าน NAT ตัวเดียว
	ImagePullPolicy string

	// RequestTimeoutSeconds = timeout ต่อ 1 คำสั่งที่ยิงไป Kubernetes API
	RequestTimeoutSeconds int
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

		ResendAPIKey:         getEnv("RESEND_API_KEY", ""),
		MailFrom:             getEnv("MAIL_FROM", "Caesar Cluster <onboarding@resend.dev>"),
		ResetTokenTTLMinutes: getEnvInt("RESET_TOKEN_TTL_MINUTES", 30),

		K8s: K8sConfig{
			PodCIDR:               getEnv("K8S_POD_CIDR", "172.16.0.0/16"),
			BlockedEgressCIDRs:    getEnvList("K8S_BLOCKED_EGRESS_CIDRS", "192.168.100.0/24"),
			DefaultContainerPort:  getEnvInt("K8S_DEFAULT_CONTAINER_PORT", 8080),
			PodSecurity:           getEnv("K8S_POD_SECURITY", PodSecurityBaseline),
			ImagePullPolicy:       getEnv("K8S_IMAGE_PULL_POLICY", "IfNotPresent"),
			RequestTimeoutSeconds: getEnvInt("K8S_REQUEST_TIMEOUT_SECONDS", 30),
		},

		AllowedImageRegistries: getEnvList("ALLOWED_IMAGE_REGISTRIES", ""),

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

	if cfg.K8s.PodSecurity != PodSecurityBaseline && cfg.K8s.PodSecurity != PodSecurityRestricted {
		log.Printf("ค่า K8S_POD_SECURITY=%q ไม่รู้จัก ใช้ %q แทน", cfg.K8s.PodSecurity, PodSecurityBaseline)
		cfg.K8s.PodSecurity = PodSecurityBaseline
	}

	// อีเมลไม่ใช่ค่าที่ทั้งระบบต้องมีถึงจะ start ได้ (ต่างจาก DB/JWT) — แค่เตือนถ้าลืมตั้ง
	// เพราะจะกระทบเฉพาะฟีเจอร์รีเซ็ตรหัสผ่าน ไม่ควรบล็อกทั้ง server สำหรับคนที่ dev ส่วนอื่นอยู่
	if cfg.ResendAPIKey == "" {
		log.Println("คำเตือน: ไม่ได้ตั้ง RESEND_API_KEY — ระบบส่งอีเมลรีเซ็ตรหัสผ่านจะยังใช้งานไม่ได้")
	}
	if cfg.IsProduction() && len(cfg.AllowedImageRegistries) == 0 {
		log.Println("คำเตือน: ไม่ได้ตั้ง ALLOWED_IMAGE_REGISTRIES — ผู้ใช้ deploy image อะไรก็ได้ขึ้นคลัสเตอร์")
	}
	return cfg
}

// IsProduction บอกว่ากำลังรันในโหมดเครื่องจริงหรือไม่ — ใช้คุม CORS และความเข้มของ config check
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

// getEnvList อ่าน env ที่เป็นรายการคั่นด้วย comma แล้วคืนเป็น slice (ตัดช่องว่าง/ตัวว่างทิ้ง)
// คืน slice ว่างเมื่อ env และ fallback ว่างทั้งคู่ — ฝั่งที่ใช้จะได้เช็ค len() ได้ตรงไปตรงมา
func getEnvList(key, fallback string) []string {
	parts := strings.Split(getEnv(key, fallback), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
