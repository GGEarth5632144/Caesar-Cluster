package config

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/entity"
)

// ConnectDB เปิด connection ด้วย GORM แล้ว AutoMigrate schema ให้ทันที
// schema มาจาก struct tag ใน entity/ ล้วนๆ (ไม่มีไฟล์ .sql แล้ว)
//
// data flow: รับ dbURL (มาจาก config) → เปิด pool → AutoMigrate ทุกตาราง
// → เพิ่ม FK ที่ AutoMigrate ไม่สร้างให้ → คืน *gorm.DB ให้ทุก layer ใช้ query
// ถ้าขั้นไหนพังจะ log.Fatal ทันที (fail fast) — server ไม่ควรขึ้นถ้า schema ยังไม่พร้อม
//
// ลำดับของ AutoMigrate สำคัญ: roles/eligible_students ต้องมาก่อน users (users อ้างถึงทั้งคู่)
// namespaces/request_templates ต้องมาก่อน services (services อ้างทั้งคู่)
// ipc_monitors ต้องมาก่อน user_containers (user_containers อ้าง ipc_id)
func ConnectDB(dbURL string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	log.Println("database connected ✓")

	if err := db.AutoMigrate(
		&entity.Role{},
		&entity.EligibleStudent{},
		&entity.User{},
		&entity.Namespace{},
		&entity.RequestTemplate{},
		&entity.Service{},
		&entity.IPCMonitor{},
		&entity.UserContainer{},
		&entity.Request{},
		&entity.UserAlert{},
		&entity.SystemAlert{},
		&entity.AuditLog{},
	); err != nil {
		log.Fatalf("automigrate failed: %v", err)
	}
	log.Println("schema migrated (AutoMigrate) ✓")

	// ไม่ประกาศ relation ให้ GORM จัดการ FK เอง เพราะเคยเจอว่ามันสร้าง sequence ผิดให้ column ที่เป็น FK
	// (เข้าใจผิดว่าเป็น auto-increment) เลยมาเพิ่ม FK เองด้วย raw SQL — idempotent รันซ้ำได้ทุกครั้งที่ start
	if err := addForeignKeys(db); err != nil {
		log.Fatalf("add foreign keys failed: %v", err)
	}
	log.Println("foreign keys ensured ✓")

	return db
}

// addForeignKeys เพิ่ม FK ทั้งหมดแบบ idempotent (เช็คก่อนว่ามี constraint ชื่อนี้แล้วหรือยัง ค่อย ALTER)
// data flow: อ่าน pg_constraint เพื่อดูว่ามีอยู่แล้วไหม → ถ้ายังไม่มีค่อย ALTER TABLE ... ADD CONSTRAINT
//
// FK ที่สำคัญที่สุดคือ users.student_id → eligible_students.student_id:
// มันบังคับกฎ "สมัครได้เฉพาะ นศ. ที่อยู่ในรายชื่อ" ที่ระดับ DB ต่อให้โค้ดลืมเช็คก็ยัง insert ไม่ผ่าน
//
// users.namespace_id กับ namespaces.contributor_id อ้างถึงกันไปมา (วงกลม) — ไม่เป็นไรใน Postgres
// เพราะตอนใช้จริงเราสร้าง user ก่อน (namespace_id = NULL) แล้วค่อยสร้าง namespace แล้วค่อยอัปเดตกลับ
func addForeignKeys(db *gorm.DB) error {
	fks := []struct {
		name string
		ddl  string
	}{
		{
			name: "fk_users_role_id",
			ddl: `ALTER TABLE users ADD CONSTRAINT fk_users_role_id
			      FOREIGN KEY (role_id) REFERENCES roles(id)`,
		},
		{
			name: "fk_users_student_id",
			ddl: `ALTER TABLE users ADD CONSTRAINT fk_users_student_id
			      FOREIGN KEY (student_id) REFERENCES eligible_students(student_id)`,
		},
		{
			// ลบ namespace ทิ้ง → สมาชิกไม่ถูกลบตาม แค่หลุดออกจาก space (namespace_id = NULL)
			name: "fk_users_namespace_id",
			ddl: `ALTER TABLE users ADD CONSTRAINT fk_users_namespace_id
			      FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON DELETE SET NULL`,
		},
		{
			name: "fk_namespaces_contributor_id",
			ddl: `ALTER TABLE namespaces ADD CONSTRAINT fk_namespaces_contributor_id
			      FOREIGN KEY (contributor_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// ลบ namespace → service ข้างในหายตามทั้งหมด (โควตาถูกคืนไปในตัว)
			name: "fk_services_namespace_id",
			ddl: `ALTER TABLE services ADD CONSTRAINT fk_services_namespace_id
			      FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON DELETE CASCADE`,
		},
		{
			name: "fk_services_created_by",
			ddl: `ALTER TABLE services ADD CONSTRAINT fk_services_created_by
			      FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// template ถูกลบ → service ยังอยู่ได้ (มี snapshot cpu/ram ของตัวเองอยู่แล้ว) แค่ request_template_id เป็น NULL
			name: "fk_services_request_template_id",
			ddl: `ALTER TABLE services ADD CONSTRAINT fk_services_request_template_id
			      FOREIGN KEY (request_template_id) REFERENCES request_templates(id) ON DELETE SET NULL`,
		},
		{
			// ลบ namespace → แถว monitoring ของ container ที่เคยอยู่ในนั้นหายตามไปด้วย
			name: "fk_user_containers_namespace_id",
			ddl: `ALTER TABLE user_containers ADD CONSTRAINT fk_user_containers_namespace_id
			      FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON DELETE CASCADE`,
		},
		{
			// ลบเครื่อง IPC ออกจากระบบ → แถว monitoring ของ container ที่เคยรันอยู่บนเครื่องนั้นหายตามไปด้วย
			name: "fk_user_containers_ipc_id",
			ddl: `ALTER TABLE user_containers ADD CONSTRAINT fk_user_containers_ipc_id
			      FOREIGN KEY (ipc_id) REFERENCES ipc_monitors(id) ON DELETE CASCADE`,
		},
		{
			name: "fk_user_alerts_user_id",
			ddl: `ALTER TABLE user_alerts ADD CONSTRAINT fk_user_alerts_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			name: "fk_requests_user_id",
			ddl: `ALTER TABLE requests ADD CONSTRAINT fk_requests_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		// audit_logs และ system_alerts ไม่มี FK ตั้งใจ — เก็บเป็น snapshot ล้วนๆ (ดูเหตุผลใน audit_log.go)
		// request_templates ไม่มี FK ไป users — เป็น choice กลาง ไม่ผูกกับ user คนใดคนหนึ่ง (ดูเหตุผลใน request_template.go)
	}

	for _, fk := range fks {
		var exists bool
		err := db.Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = ?
		)`, fk.name).Scan(&exists).Error
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := db.Exec(fk.ddl).Error; err != nil {
			return err
		}
	}
	return nil
}
