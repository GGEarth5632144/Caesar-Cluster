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

// seed ยัดข้อมูลตั้งต้นที่ระบบต้องมีถึงจะทำงานได้ แยกจาก AutoMigrate โดยตั้งใจ
// (schema เกิดตอน server start เสมอ ส่วน seed data รันเองเมื่อต้องการ)
//
// ทุกขั้น idempotent — รันซ้ำได้ไม่พัง ไม่เกิดข้อมูลซ้ำ
//
// data flow: config.Load → ConnectDB (สร้าง schema ให้ก่อน) → เขียน 3 อย่างลง DB:
//  1. roles (user, admin)         — Register ต้องใช้ role "user" ไม่งั้นสมัครไม่ได้
//  2. admin คนแรก                 — ต้องมีแถวใน eligible_students ก่อน เพราะติด FK
//  3. plans ตั้งต้น (choices)      — ให้ผู้ใช้มีตัวเลือกใช้ตั้งแต่แรก admin เพิ่มทีหลังได้
//
// รัน: go run ./cmd/seed
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl)

	seedRoles(db)
	seedAdmin(db)
	seedPlans(db)

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

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash ไม่สำเร็จ: %v", err)
	}

	admin := entity.User{
		StudentID: adminStudentID,
		RoleID:    adminRole.ID,
		RealName:  "System Admin",
		NickName:  "admin",
		Password:  string(hash),
	}
	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("seed admin ไม่สำเร็จ: %v", err)
	}

	log.Printf("สร้าง admin เริ่มต้นแล้ว — student_id=%s password=%s", adminStudentID, adminPassword)
	log.Println("*** เปลี่ยนรหัสผ่านทันทีหลัง login ครั้งแรก ***")
}

// seedPlans ใส่ choices ตั้งต้นให้ผู้ใช้เลือกตอน deploy
//
// data flow: INSERT plans แบบ ON CONFLICT DO NOTHING (ชนกันที่ name) → admin เพิ่ม/แก้เองทีหลังได้
// สเปกสูงสุดที่ตั้งได้คือ 3000m/2048MB ซึ่งเท่ากับเพดานของ service 1 ตัวพอดี (300% / 2 GB)
func seedPlans(db *gorm.DB) {
	plans := []entity.Plan{
		{Name: "small", CPUMilli: 500, RAMMB: 512, IsActive: true},    // 50%
		{Name: "medium", CPUMilli: 1000, RAMMB: 1024, IsActive: true}, // 100%
		{Name: "large", CPUMilli: 2000, RAMMB: 2048, IsActive: true},  // 200%
		{Name: "max", CPUMilli: 3000, RAMMB: 2048, IsActive: true},    // 300% = เพดานต่อ service
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&plans).Error; err != nil {
		log.Fatalf("seed plans ไม่สำเร็จ: %v", err)
	}
	log.Println("plans ตั้งต้นพร้อมแล้ว (small, medium, large, max) ✓")
}
