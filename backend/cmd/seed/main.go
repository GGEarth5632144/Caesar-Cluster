package main

import (
	"log"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/config"
	"backend/internal/entity"
)

// รหัสผ่านตั้งต้นของ admin — ต้องเปลี่ยนทันทีหลัง login ครั้งแรก
const adminStudentID = "admin"
const adminPassword = "changeme123"

const StudentID = "B6618452"
const userPassword = "Banana1234"

// seed ยัดข้อมูลตั้งต้นที่ระบบต้องมีถึงจะทำงานได้ แยกจาก AutoMigrate โดยตั้งใจ
// (schema เกิดตอน server start เสมอ ส่วน seed data รันเองเมื่อต้องการ)
//
// ทุกขั้น idempotent — รันซ้ำได้ไม่พัง ไม่เกิดข้อมูลซ้ำ
//
// data flow: config.Load → ConnectDB (สร้าง schema ให้ก่อน) → เขียน 3 อย่างลง DB:
//  1. roles (user, admin)                — Register ต้องใช้ role "user" ไม่งั้นสมัครไม่ได้
//  2. admin คนแรก                        — ต้องมีแถวใน eligible_students ก่อน เพราะติด FK
//  3. request_templates ตั้งต้น (choices) — ให้ผู้ใช้มีตัวเลือกใช้ตั้งแต่แรก admin เพิ่มทีหลังได้
//
// รัน: go run ./cmd/seed
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl)

	seedRoles(db)
	seedAdmin(db)
	seeduser(db)
	seedRequestTemplates(db)

	log.Println("seed เสร็จแล้ว ✓")
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

// seedAdmin สร้าง admin คนแรกของระบบ (ข้ามถ้ามี admin อยู่แล้ว)
//
// data flow: COUNT users ที่ role = admin → ถ้ามีแล้วข้าม
// → ถ้ายังไม่มี: INSERT eligible_students ก่อน (เพราะ users.student_id มี FK ชี้มาที่ตารางนี้)
// → hash password → INSERT users พร้อม role_id ของ admin
//
// ลำดับสำคัญ: ข้าม eligible_students ไม่ได้ ต่อให้เป็น admin ก็ต้องอยู่ในรายชื่อ (กฎเดียวกันทั้งระบบ)
func seedAdmin(db *gorm.DB) {
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
	eligible := entity.EligibleStudent{StudentID: adminStudentID, Major: "System"}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&eligible).Error; err != nil {
		log.Fatalf("seed eligible admin ไม่สำเร็จ: %v", err)
	}

	hashadmin, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash ไม่สำเร็จ: %v", err)
	}

	admin := entity.User{
		StudentID: adminStudentID,
		RoleID:    adminRole.ID,
		RealName:  "System Admin",
		NickName:  "admin",
		Password:  string(hashadmin),
	}
	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("seed admin ไม่สำเร็จ: %v", err)
	}

	log.Printf("สร้าง admin เริ่มต้นแล้ว — student_id=%s password=%s", adminStudentID, adminPassword)
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

	eligible := entity.EligibleStudent{StudentID: StudentID, Major: "CPE"}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&eligible).Error; err != nil {
		log.Fatalf("seed eligible user ไม่สำเร็จ: %v", err)
	}

	hashuser, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash ไม่สำเร็จ: %v", err)
	}
	user := entity.User{
		StudentID: StudentID,
		RoleID:    userRole.ID,
		RealName:  "Nattanant",
		NickName:  "Earth",
		Password:  string(hashuser),
	}
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("seed user ไม่สำเร็จ: %v", err)
	}

	log.Printf("สร้าง user เริ่มต้นแล้ว — student_id=%s password=%s", StudentID, userPassword)
	log.Println("*** เปลี่ยนรหัสผ่านทันทีหลัง login ครั้งแรก ***")
}
// seedRequestTemplates ใส่ choices ตั้งต้นให้ผู้ใช้เลือกตอนยื่น request หรือ deploy service
//
// data flow: INSERT request_templates แบบ ON CONFLICT DO NOTHING (ชนกันที่ name) → admin เพิ่ม/แก้เองทีหลังได้
// สเปกสูงสุดที่ตั้งได้คือ 3000m/2048MB ซึ่งเท่ากับเพดานของ service 1 ตัวพอดี (300% / 2 GB)
func seedRequestTemplates(db *gorm.DB) {
	templates := []entity.RequestTemplate{
		{Name: "small", CPULimitMilli: 500, RAMLimitMB: 512, IsActive: true},    // 50%
		{Name: "medium", CPULimitMilli: 1000, RAMLimitMB: 1024, IsActive: true}, // 100%
		{Name: "large", CPULimitMilli: 2000, RAMLimitMB: 2048, IsActive: true},  // 200%
		{Name: "max", CPULimitMilli: 3000, RAMLimitMB: 2048, IsActive: true},    // 300% = เพดานต่อ service
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&templates).Error; err != nil {
		log.Fatalf("seed request templates ไม่สำเร็จ: %v", err)
	}
	log.Println("request templates ตั้งต้นพร้อมแล้ว (small, medium, large, max) ✓")
}
