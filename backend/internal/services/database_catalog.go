package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"backend/internal/entity"
)

// ── catalog ของ database template (docs 029) ─────────────────────────────────────────
//
// แหล่งความจริงเดียวของ engine/version ที่ผู้ใช้ deploy ได้ — หน้าเว็บดึงผ่าน GET /api/database-templates
// ไม่เขียนรายการซ้ำฝั่ง frontend เพราะวัน EOL ต้องตัดสินที่เดียว (ไม่งั้นหน้าเว็บโชว์ version ที่ API ไม่รับ)
//
// ทุก image ในนี้ทดสอบบน worker จริงแล้ว (2026-09-17): Atom E3950 ไม่มี AVX จึงรัน image บางตัวไม่ได้
// (MongoDB ≥5.0) — เพิ่ม version ใหม่ต้องทดสอบบน worker ก่อนเสมอ ไม่ใช่แค่ดูว่า tag มีอยู่

// DatabaseVersion = version หนึ่งของ engine
type DatabaseVersion struct {
	Version string `json:"version"`
	// EOL = วันสุดท้ายที่ upstream ยัง support (YYYY-MM-DD) — เลยวันนี้แล้วเลือก deploy ใหม่ไม่ได้
	// ส่วน database ที่ deploy ไปแล้วยังรันต่อได้ตามปกติ
	EOL     string `json:"eol"`
	LTS     bool   `json:"lts"`
	Default bool   `json:"default"`

	image    string // ใช้ tag ระดับ major/LTS ไม่ใช่ latest — ได้ patch อัตโนมัติแต่ไม่ข้ามรุ่นเอง
	dataPath string // PostgreSQL 18 ย้ายจุด mount เป็น /var/lib/postgresql (≤17 ใช้ .../data)
}

// DatabaseEngine = engine หนึ่งตัวพร้อมกติกาที่ต่างกันต่อ engine
type DatabaseEngine struct {
	Engine   string            `json:"engine"`
	Label    string            `json:"label"`
	Port     int               `json:"port"`
	MinRAMMB int               `json:"min_ram_mb"`
	Versions []DatabaseVersion `json:"versions"`

	scheme   string // scheme ของ connection URL
	sslParam string // query ที่ปิด TLS — ชื่อต่างกันต่อ driver จึงเลือกตัวที่ driver ส่วนใหญ่ของ engine นั้นรู้จัก

	// ชื่อ env ของ image ทางการที่อ่านค่าจาก Secret — image อ่านค่าเหล่านี้ครั้งเดียวตอนสร้างข้อมูลครั้งแรก
	envUser, envPassword, envDatabase, envRootPassword string
}

const (
	EnginePostgreSQL = "postgresql"
	EngineMySQL      = "mysql"
	EngineMariaDB    = "mariadb"
)

// databaseCatalog = ข้อมูล EOL จาก endoflife.date ณ 2026-09-16
//
// MinRAMMB มาจากการวัดบน worker: PostgreSQL ว่างๆ ใช้ ~70–90 MB, MariaDB ~110–150 MB,
// MySQL 8.4/9.7 ใช้หน่วยความจำจริง ~430 MB ตั้งแต่ยังไม่มีงาน (ที่ 512 MB ชนเพดานพอดี)
var databaseCatalog = []DatabaseEngine{
	{
		Engine: EnginePostgreSQL, Label: "PostgreSQL", Port: 5432, MinRAMMB: 256,
		scheme: "postgresql", sslParam: "sslmode=disable",
		envUser: "POSTGRES_USER", envPassword: "POSTGRES_PASSWORD", envDatabase: "POSTGRES_DB",
		Versions: []DatabaseVersion{
			{Version: "18", EOL: "2030-11-14", Default: true, image: "postgres:18", dataPath: "/var/lib/postgresql"},
			{Version: "17", EOL: "2029-11-08", image: "postgres:17", dataPath: "/var/lib/postgresql/data"},
			{Version: "16", EOL: "2028-11-09", image: "postgres:16", dataPath: "/var/lib/postgresql/data"},
			{Version: "15", EOL: "2027-11-11", image: "postgres:15", dataPath: "/var/lib/postgresql/data"},
			{Version: "14", EOL: "2026-11-12", image: "postgres:14", dataPath: "/var/lib/postgresql/data"},
		},
	},
	{
		Engine: EngineMySQL, Label: "MySQL", Port: 3306, MinRAMMB: 768,
		scheme: "mysql", sslParam: "ssl=false",
		envUser: "MYSQL_USER", envPassword: "MYSQL_PASSWORD", envDatabase: "MYSQL_DATABASE",
		envRootPassword: "MYSQL_ROOT_PASSWORD",
		// เฉพาะ LTS — innovation release อายุสั้นไม่กี่เดือน
		Versions: []DatabaseVersion{
			{Version: "8.4", EOL: "2032-04-30", LTS: true, Default: true, image: "mysql:8.4", dataPath: "/var/lib/mysql"},
			{Version: "9.7", EOL: "2034-04-21", LTS: true, image: "mysql:9.7", dataPath: "/var/lib/mysql"},
		},
	},
	{
		Engine: EngineMariaDB, Label: "MariaDB", Port: 3306, MinRAMMB: 256,
		scheme: "mysql", sslParam: "ssl=false",
		envUser: "MARIADB_USER", envPassword: "MARIADB_PASSWORD", envDatabase: "MARIADB_DATABASE",
		envRootPassword: "MARIADB_ROOT_PASSWORD",
		Versions: []DatabaseVersion{
			{Version: "11.8", EOL: "2028-06-04", LTS: true, Default: true, image: "mariadb:11.8", dataPath: "/var/lib/mysql"},
			{Version: "12.3", EOL: "2029-06-12", LTS: true, image: "mariadb:12.3", dataPath: "/var/lib/mysql"},
			{Version: "11.4", EOL: "2029-05-29", LTS: true, image: "mariadb:11.4", dataPath: "/var/lib/mysql"},
			{Version: "10.11", EOL: "2028-02-16", LTS: true, image: "mariadb:10.11", dataPath: "/var/lib/mysql"},
		},
	},
}

// Secret ของ database: ชื่อ key เหมือนกันทุก engine แล้ว catalog จับคู่ไปเป็นชื่อ env ของแต่ละ image
const (
	SecretKeyUsername     = "username"
	SecretKeyPassword     = "password"
	SecretKeyDatabase     = "database"
	SecretKeyRootPassword = "root-password"
)

var (
	ErrDatabaseEngineNotFound  = errors.New("ไม่มี database engine นี้ให้เลือก")
	ErrDatabaseVersionNotFound = errors.New("version นี้ไม่มีให้เลือก หรือหมดระยะ support แล้ว")
	ErrDatabaseInvalidInput    = errors.New("ข้อมูลของ database ไม่ถูกต้อง")
	ErrDatabaseRAMTooLow       = errors.New("RAM น้อยเกินไปสำหรับ database นี้")

	// database template ผูก image/version/credential ไว้กับข้อมูลที่ image สร้างครั้งแรก — แก้แล้วไม่มีผลจริง
	ErrDatabaseImmutable = errors.New("database template แก้ได้เฉพาะ CPU/RAM — ชื่อ, image, version และ credential แก้หลัง deploy ไม่ได้")

	// สวิตช์ "ใช้ service นี้เป็นฐานข้อมูล" ถูกถอดแล้ว — database ต้องมาจาก template เท่านั้น
	ErrDatabaseUseTemplate = errors.New("สร้าง database ผ่าน POST /api/databases (เลือกจาก template) เท่านั้น")

	// database ยุคก่อน template (image อิสระ) ไม่มี credential ที่ระบบรู้จัก
	ErrNotTemplateDatabase = errors.New("service นี้ไม่ใช่ database ที่สร้างจาก template จึงไม่มีข้อมูลการเชื่อมต่อ")
)

// supportedOn = version นี้ยังเลือก deploy ได้ ณ เวลา now (ใช้ได้ถึงสิ้นวัน EOL ตามเวลา UTC)
func (v DatabaseVersion) supportedOn(now time.Time) bool {
	eol, err := time.Parse(time.DateOnly, v.EOL)
	if err != nil {
		return false
	}
	return now.UTC().Before(eol.AddDate(0, 0, 1))
}

// DatabaseTemplates คืน catalog เฉพาะ version ที่ยัง support ณ now (engine ที่ไม่เหลือ version เลยถูกตัดออก)
// ถ้า version ที่ตั้งเป็น default หมดอายุ ตัวบนสุดที่เหลือจะเป็น default แทน
func DatabaseTemplates(now time.Time) []DatabaseEngine {
	out := make([]DatabaseEngine, 0, len(databaseCatalog))
	for _, e := range databaseCatalog {
		e.Versions = slices.DeleteFunc(slices.Clone(e.Versions), func(v DatabaseVersion) bool { return !v.supportedOn(now) })
		if len(e.Versions) == 0 {
			continue
		}
		if !slices.ContainsFunc(e.Versions, func(v DatabaseVersion) bool { return v.Default }) {
			e.Versions[0].Default = true
		}
		out = append(out, e)
	}
	return out
}

// findDatabaseEngine หา engine ใน catalog (รวม version ที่หมด support แล้ว — database เก่ายังต้องหา env/URL ได้)
func findDatabaseEngine(engine string) (DatabaseEngine, bool) {
	i := slices.IndexFunc(databaseCatalog, func(e DatabaseEngine) bool { return e.Engine == engine })
	if i < 0 {
		return DatabaseEngine{}, false
	}
	return databaseCatalog[i], true
}

// resolveDatabaseVersion หา engine + version สำหรับ deploy ใหม่ — version ว่าง = ใช้ default
func resolveDatabaseVersion(engine, version string, now time.Time) (DatabaseEngine, DatabaseVersion, error) {
	templates := DatabaseTemplates(now)
	i := slices.IndexFunc(templates, func(e DatabaseEngine) bool { return e.Engine == engine })
	if i < 0 {
		return DatabaseEngine{}, DatabaseVersion{}, ErrDatabaseEngineNotFound
	}
	e := templates[i]
	for _, v := range e.Versions {
		if v.Version == version || (version == "" && v.Default) {
			return e, v, nil
		}
	}
	return DatabaseEngine{}, DatabaseVersion{}, fmt.Errorf("%w: %s %s", ErrDatabaseVersionNotFound, e.Label, version)
}

// ── ตรวจ credential ที่ผู้ใช้กรอก ────────────────────────────────────────────────────

var (
	// ตัวพิมพ์เล็กล้วน: PostgreSQL แปลงชื่อที่ไม่ quote เป็นตัวเล็ก ผู้ใช้ที่ตั้ง "App" จะ login ไม่ติดใน client บางตัว
	dbUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{2,31}$`)
	dbNamePattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
)

// ชื่อที่ image สร้างไว้เองหรือเป็นของระบบ — ใช้ซ้ำแล้ว entrypoint ล้มหรือได้สิทธิ์ผิดจากที่คิด
var (
	reservedDBUsernames = []string{"root", "mysql", "mariadb", "information_schema", "performance_schema", "sys"}
	reservedDBNames     = map[string][]string{
		EnginePostgreSQL: {"template0", "template1"},
		EngineMySQL:      {"mysql", "information_schema", "performance_schema", "sys"},
		EngineMariaDB:    {"mysql", "information_schema", "performance_schema", "sys"},
	}
)

// validateDatabaseCredentials คืนข้อความอธิบาย สตริงว่าง = ผ่าน
//
// รหัสผ่านห้ามมีช่องว่าง ' " ` \ — ค่าส่งผ่าน Secret (ไม่ผ่าน shell) แต่ entrypoint ของ image
// เอาไปประกอบ SQL และ client/driver หลายตัว parse อักขระพวกนี้ต่างกัน ตัดทิ้งดีกว่าให้ login ไม่ติดแบบเดาไม่ออก
func validateDatabaseCredentials(engine, username, password, database string) string {
	switch {
	case !dbUsernamePattern.MatchString(username):
		return "username ต้องเป็น a-z, 0-9, _ ยาว 3–32 ตัว และไม่ขึ้นต้นด้วยตัวเลข"
	case slices.Contains(reservedDBUsernames, username):
		return "username '" + username + "' เป็นชื่อที่ระบบใช้อยู่แล้ว"
	case engine == EnginePostgreSQL && strings.HasPrefix(username, "pg_"):
		return "PostgreSQL ห้ามตั้ง username ขึ้นต้นด้วย pg_"
	case len(password) < 8 || len(password) > 64:
		return "password ต้องยาว 8–64 ตัวอักษร"
	case strings.ContainsFunc(password, func(r rune) bool { return r < 0x21 || r > 0x7e || strings.ContainsRune(`'"`+"`"+`\`, r) }):
		return "password ใช้ได้เฉพาะตัวอักษรภาษาอังกฤษ ตัวเลข และสัญลักษณ์ (ห้ามช่องว่าง ' \" ` \\)"
	case !dbNamePattern.MatchString(database):
		return "ชื่อ database ต้องเป็น A-Z, a-z, 0-9, _ ยาวไม่เกิน 63 ตัว และไม่ขึ้นต้นด้วยตัวเลข"
	case slices.Contains(reservedDBNames[engine], strings.ToLower(database)):
		return "ชื่อ database '" + database + "' เป็นของระบบ"
	}
	return ""
}

// randomDatabasePassword สุ่มรหัส root ที่ไม่มีใครต้องพิมพ์ (hex ล้วน = ไม่มีอักขระให้ escape)
func randomDatabasePassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ── การเชื่อมต่อ ─────────────────────────────────────────────────────────────────────

// DatabaseCredentials = ค่าที่อ่านกลับจาก Secret ของ database
type DatabaseCredentials struct {
	Username string
	Password string
	Database string
}

// DatabaseConnection = คำตอบของ GET /api/services/:id/connection
type DatabaseConnection struct {
	Engine   string `json:"engine"`
	Version  string `json:"version"`
	Host     string `json:"host"` // ชื่อ service สั้นๆ — ใช้ได้เฉพาะ pod ใน namespace เดียวกัน
	FQDN     string `json:"fqdn"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	URL      string `json:"url"`
}

// buildConnectionURL ประกอบ URL ด้วย net/url เพื่อ encode อักขระพิเศษในรหัสผ่าน (@ : / # ?)
// ต่อสตริงเองแล้วรหัสที่มี @ จะทำให้ driver ตัด host ผิดที่
func buildConnectionURL(e DatabaseEngine, host string, cred DatabaseCredentials) string {
	u := url.URL{
		Scheme:   e.scheme,
		User:     url.UserPassword(cred.Username, cred.Password),
		Host:     host + ":" + strconv.Itoa(e.Port),
		Path:     "/" + cred.Database,
		RawQuery: e.sslParam,
	}
	return u.String()
}

// newDatabaseConnection ประกอบคำตอบของ connection endpoint จาก service + credential ใน Secret
func newDatabaseConnection(svc *entity.Service, nsName string, cred DatabaseCredentials) (DatabaseConnection, error) {
	e, ok := findDatabaseEngine(svc.DatabaseEngine)
	if !ok {
		return DatabaseConnection{}, ErrNotTemplateDatabase
	}
	return DatabaseConnection{
		Engine:   e.Engine,
		Version:  svc.DatabaseVersion,
		Host:     svc.Name,
		FQDN:     svc.Name + "." + nsName + ".svc.cluster.local",
		Port:     e.Port,
		Username: cred.Username,
		Password: cred.Password,
		Database: cred.Database,
		URL:      buildConnectionURL(e, svc.Name, cred),
	}, nil
}
