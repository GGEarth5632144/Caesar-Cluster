package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/config"
	"backend/internal/entity"
)

// รหัสผ่านตั้งต้นของ admin — ต้องเปลี่ยนทันทีหลัง login ครั้งแรก
const adminStudentID = "admin"
const devAdminPassword = "changeme123"

const StudentID = "B6618452"
const userPassword = "Banana1234"

// seed ยัดข้อมูลตั้งต้นที่ระบบต้องมีถึงจะทำงานได้ (รัน: go run ./cmd/seed) ทุกขั้น idempotent
// แยกจาก AutoMigrate โดยตั้งใจ — schema เกิดตอน server start เสมอ ส่วน seed data รันเองเมื่อต้องการ
//
// roles / admin คนแรก / request_templates ระบบขาดไม่ได้ จึงรันทุกครั้ง
// ส่วนบัญชี + รายชื่อทดสอบมีรหัสผ่านเขียนตายไว้ในซอร์ส จึงกั้นด้วย SEED_DEMO_DATA
// ที่ปิดเองเมื่อ APP_ENV=production — กั้นแทนลบทิ้งเพราะยังใช้ทดสอบ flow สมัครบนเครื่อง dev อยู่จริง
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl)

	seedRoles(db)
	seedAdmin(db, cfg)
	seedRequestTemplates(db)

	if !seedDemoData(cfg) {
		log.Println("ข้ามข้อมูลสาธิต (บัญชีทดสอบ + รายชื่อ eligible ปลอม) — " +
			"ตั้ง SEED_DEMO_DATA=true ถ้าต้องการจริงๆ")
		log.Println("seed เสร็จแล้ว ✓")
		return
	}
	seeduser(db)
	seedTestEligibleStudents(db)

	log.Println("seed เสร็จแล้ว ✓")
}

// seedDemoData ตัดสินว่าจะใส่ข้อมูลสาธิตหรือไม่
//
// ค่าปกติ: ใส่ตอน dev, ไม่ใส่ตอน production — ตั้ง SEED_DEMO_DATA ทับได้ทั้งสองทาง
// (เผื่ออยากได้เครื่อง staging ที่มีข้อมูลทดสอบครบ หรืออยากได้เครื่อง dev ที่สะอาด)
func seedDemoData(cfg *config.Config) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SEED_DEMO_DATA"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return !cfg.IsProduction()
	}
}

// adminPasswordFor เลือกรหัสผ่านตั้งต้นของ admin
//
// บนเครื่องจริงห้ามใช้ค่าที่เขียนไว้ในซอร์ส — ใครอ่าน repo ก็ล็อกอินเป็นแอดมินได้ทันที
// จึงบังคับให้ตั้ง SEED_ADMIN_PASSWORD เอง และไม่ยอมให้ตั้งซ้ำกับค่าตัวอย่าง
// (หลักการเดียวกับที่ config.Load ไม่ยอมให้ production ใช้ JWT_SECRET=dev-secret)
func adminPasswordFor(cfg *config.Config) string {
	pw := os.Getenv("SEED_ADMIN_PASSWORD")
	if !cfg.IsProduction() {
		if pw == "" {
			return devAdminPassword
		}
		return pw
	}
	if pw == "" || pw == devAdminPassword || len(pw) < 12 {
		log.Fatal("APP_ENV=production ต้องตั้ง SEED_ADMIN_PASSWORD เป็นรหัสผ่านของตัวเอง " +
			"ยาวอย่างน้อย 12 ตัวอักษร และห้ามใช้ค่าตัวอย่างในซอร์ส")
	}
	return pw
}

// seedRoles ใส่ role ตั้งต้น (user, admin) ลงตาราง roles
// data flow: INSERT roles แบบ ON CONFLICT DO NOTHING → ถ้ามีอยู่แล้วข้ามไป
// role พวกนี้จำเป็นมาก: AuthController.Register หา role "user" ไม่เจอจะสมัครไม่ได้เลย
func seedRoles(db *gorm.DB) {
	roles := []entity.Role{
		{Name: entity.RoleUser},
		{Name: entity.RoleAdmin},
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&roles).Error; err != nil {
		log.Fatalf("seed roles ไม่สำเร็จ: %v", err)
	}
	log.Println("roles พร้อมแล้ว (user, admin) ✓")
}

func seedRequestTemplates(db *gorm.DB) {
	templates := []entity.RequestTemplate{
		{
			OptionName:    "Cyber Range Node",
			Category:      "Security",
			Description:   "เครื่องจำลองเครือข่ายสำหรับทดสอบความปลอดภัยไซเบอร์",
			RelateSubject: "Cyber Security",
			CPULimitMilli: 2000, // 2 Core
			RAMLimitMB:    4096, // 4 GB
			StorageGB:     20,   // 20 GB
			IsActive:      false,
		},
		{
			OptionName:    "AI Vision Model",
			Category:      "Deep Learning",
			Description:   "เครื่องสเปกสูงสำหรับเทรนโมเดล YOLO และ Vision Transformers",
			RelateSubject: "Deep Learning",
			CPULimitMilli: 4000, // 4 Core
			RAMLimitMB:    8192, // 8 GB
			StorageGB:     50,   // 50 GB
			IsActive:      false,
		},
		{
			OptionName:    "Basic Web Service",
			Category:      "Web",
			Description:   "เครื่องสำหรับรัน Full-Stack Web (React, Go, Node.js)",
			RelateSubject: "System Analysis",
			CPULimitMilli: 1000, // 1 Core
			RAMLimitMB:    2048, // 2 GB
			StorageGB:     15,   // 15 GB
			IsActive:      false,
		},
	}

	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&templates).Error; err != nil {
		log.Fatalf("seed request templates ไม่สำเร็จ: %v", err)
	}
	log.Println("request templates พร้อมแล้ว [OK]")
}

// seedAdmin สร้าง admin คนแรกของระบบ (ข้ามถ้ามีอยู่แล้ว)
// ต้อง INSERT eligible_students ก่อนเสมอ เพราะ users.student_id มี FK ชี้มาที่ตารางนั้น —
// ต่อให้เป็น admin ก็ต้องอยู่ในรายชื่อ (กฎเดียวกันทั้งระบบ)
func seedAdmin(db *gorm.DB, cfg *config.Config) {
	var adminRole entity.Role
	if err := db.Where("name = ?", entity.RoleAdmin).First(&adminRole).Error; err != nil {
		log.Fatalf("หา role admin ไม่เจอ: %v", err)
	}

	var count int64
	db.Model(&entity.User{}).Where("role_id = ?", adminRole.ID).Count(&count)
	if count > 0 {
		log.Println("มี admin อยู่แล้ว ข้าม seed admin")
		return
	}

	// admin ก็ต้องอยู่ในรายชื่อผู้มีสิทธิ์เหมือนกัน (ติด FK users.student_id → eligible_students)
	eligible := entity.EligibleStudent{StudentID: adminStudentID, Major: "System", EnrollmentStatus: 10}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&eligible).Error; err != nil {
		log.Fatalf("seed eligible admin ไม่สำเร็จ: %v", err)
	}

	hashadmin, err := bcrypt.GenerateFromPassword([]byte(adminPasswordFor(cfg)), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash ไม่สำเร็จ: %v", err)
	}

	// บัญชีที่ seed สร้างเองต้องนับว่ายืนยันอีเมลแล้ว — ไม่มีใครไปกดลิงก์ยืนยันแทนได้
	// (ที่อยู่ที่ใส่ไว้ก็เป็นอีเมลสมมติ) ถ้าปล่อยเป็น NULL บัญชีที่ seed ไว้จะล็อกอินไม่ได้เลยตั้งแต่แรก
	// UTC เสมอ ด้วยเหตุผลเดียวกับ controller.dbNow (คอลัมน์เป็น timestamp without time zone)
	verifiedAt := time.Now().UTC()

	admin := entity.User{
		StudentID:       adminStudentID,
		RoleID:          adminRole.ID,
		RealName:        "System Admin",
		NickName:        "admin",
		Gmail:           "system@gmail.com",
		EntryYear:       3,
		Password:        string(hashadmin),
		GmailVerifiedAt: &verifiedAt,
	}
	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("seed admin ไม่สำเร็จ: %v", err)
	}

	log.Printf("สร้าง admin เริ่มต้นแล้ว — student_id=%s password=[hidden]", adminStudentID)
	log.Println("*** เปลี่ยนรหัสผ่านทันทีหลัง login ครั้งแรก ***")

}

func seeduser(db *gorm.DB) {
	var userRole entity.Role
	if err := db.Where("name = ?", entity.RoleUser).First(&userRole).Error; err != nil {
		log.Fatalf("หา role user ไม่เจอ: %v", err)
	}

	var count int64
	db.Model(&entity.User{}).Where("role_id = ?", userRole.ID).Count(&count)
	if count > 0 {
		log.Println("มี user อยู่แล้ว ข้าม seed user")
		return
	}

	eligible := entity.EligibleStudent{StudentID: StudentID, Major: entity.MajorCPE, EnrollmentStatus: 10}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&eligible).Error; err != nil {
		log.Fatalf("seed eligible user ไม่สำเร็จ: %v", err)
	}

	hashuser, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash ไม่สำเร็จ: %v", err)
	}
	// บัญชีที่ seed สร้างเองต้องนับว่ายืนยันอีเมลแล้ว — ไม่มีใครไปกดลิงก์ยืนยันแทนได้
	// (ที่อยู่ที่ใส่ไว้ก็เป็นอีเมลสมมติ) ถ้าปล่อยเป็น NULL บัญชีที่ seed ไว้จะล็อกอินไม่ได้เลยตั้งแต่แรก
	// UTC เสมอ ด้วยเหตุผลเดียวกับ controller.dbNow (คอลัมน์เป็น timestamp without time zone)
	verifiedAt := time.Now().UTC()

	user := entity.User{
		StudentID:       StudentID,
		RoleID:          userRole.ID,
		RealName:        "Nattanant",
		NickName:        "Earth",
		Gmail:           "nattanant563214@gmail.com",
		EntryYear:       4,
		Password:        string(hashuser),
		GmailVerifiedAt: &verifiedAt,
	}
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("seed user ไม่สำเร็จ: %v", err)
	}

	log.Printf("สร้าง user เริ่มต้นแล้ว — student_id=%s password=[hidden]", StudentID)
	log.Println("*** เปลี่ยนรหัสผ่านทันทีหลัง login ครั้งแรก ***")
}

// seedTestEligibleStudents ใส่รายชื่อ นศ. ทดสอบ B6600001-B6600011 (ON CONFLICT DO NOTHING รันซ้ำได้)
//
// จงใจใส่ major/สถานภาพไม่เหมือนกันเพื่อทดสอบด่านของ Register ได้ครบ: B6600009-B6600010 ไม่ใช่ CPE
// (NOT_CPE) และ B6600008 เป็น CPE แต่สถานภาพ 40 (NOT_ACTIVE_STUDENT)
func seedTestEligibleStudents(db *gorm.DB) {
	rows := make([]entity.EligibleStudent, 0, 10)
	for i := 1; i <= 7; i++ {
		rows = append(rows, entity.EligibleStudent{
			StudentID:        fmt.Sprintf("B66%05d", i),
			Major:            entity.MajorCPE,
			EnrollmentStatus: 10,
		})
	}
	rows = append(rows,
		entity.EligibleStudent{StudentID: "B6600008", Major: entity.MajorCPE, EnrollmentStatus: 40},
		entity.EligibleStudent{StudentID: "B6600009", Major: "Electrical Engineering", EnrollmentStatus: 10},
		entity.EligibleStudent{StudentID: "B6600010", Major: "Mechanical Engineering", EnrollmentStatus: 10},
	)

	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		log.Fatalf("seed test eligible students ไม่สำเร็จ: %v", err)
	}
	log.Println("eligible_students ทดสอบพร้อมแล้ว (B6600001-B6600007 = CPE active, B6600008 = CPE แต่จบแล้ว, B6600009-B6600010 = ไม่ใช่ CPE) ✓")
}
