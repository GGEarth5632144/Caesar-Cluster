package main

import (
	"fmt"
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
// data flow: config.Load → ConnectDB (สร้าง schema ให้ก่อน) → เขียน 4 อย่างลง DB:
//  1. roles (user, admin)                — Register ต้องใช้ role "user" ไม่งั้นสมัครไม่ได้
//  2. admin คนแรก                        — ต้องมีแถวใน eligible_students ก่อน เพราะติด FK
//  3. request_templates ตั้งต้น (choices) — ให้ผู้ใช้มีตัวเลือกใช้ตั้งแต่แรก admin เพิ่มทีหลังได้
//  4. eligible_students ทดสอบ (B6600001-B6600010) — ไว้ทดสอบ flow สมัครสมาชิกโดยไม่ต้องยิง
//     POST /api/admin/eligible-students เองก่อนทุกครั้ง
//
// รัน: go run ./cmd/seed
func main() {
	cfg := config.Load()
	db := config.ConnectDB(cfg.DBUrl)

	seedRoles(db)
	seedAdmin(db)
	seedRequestTemplates(db)
	seedTestEligibleStudents(db)

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

// seedTestEligibleStudents ใส่รายชื่อ นศ. ทดสอบ B6600001-B6600010 ลงตาราง eligible_students
// ไว้ให้ทีม frontend/QA ทดสอบหน้า Register ได้เลยโดยไม่ต้องยิง POST /api/admin/eligible-students เอง
//
// จงใจใส่ major ไม่เหมือนกันหมด (8 คนแรกเป็น CPE, 2 คนสุดท้ายไม่ใช่)
// เพื่อให้ทดสอบด่านที่ 2 ของ Register ได้ด้วย (เจอ student_id แต่ไม่ใช่ CPE → 403 NOT_CPE)
// ส่วนด่านที่ 1 (หา student_id ไม่เจอเลย) ทดสอบได้จาก student_id ไหนก็ได้ที่ไม่อยู่ใน 10 ตัวนี้
//
// data flow: INSERT eligible_students แบบ ON CONFLICT DO NOTHING (รันซ้ำได้ ไม่พัง)
func seedTestEligibleStudents(db *gorm.DB) {
	rows := make([]entity.EligibleStudent, 0, 10)
	for i := 1; i <= 8; i++ {
		rows = append(rows, entity.EligibleStudent{
			StudentID: fmt.Sprintf("B66%05d", i),
			Major:     entity.MajorCPE,
		})
	}
	rows = append(rows,
		entity.EligibleStudent{StudentID: "B6600009", Major: "Electrical Engineering"},
		entity.EligibleStudent{StudentID: "B6600010", Major: "Mechanical Engineering"},
	)

	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		log.Fatalf("seed test eligible students ไม่สำเร็จ: %v", err)
	}
	log.Println("eligible_students ทดสอบพร้อมแล้ว (B6600001-B6600008 = CPE, B6600009-B6600010 = ไม่ใช่ CPE) ✓")
}
