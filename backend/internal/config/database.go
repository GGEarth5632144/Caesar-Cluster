package config

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/entity"
)

// openDB เปิด connection pool ด้วย GORM เฉยๆ (ยังไม่ migrate) — ใช้ร่วมกันทั้ง ConnectDB
// (server/seed ต้อง migrate) และ ConnectDBReadOnly (เครื่องมือที่อ่านอย่างเดียว ไม่ควร migrate)
func openDB(dbURL string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	log.Println("database connected ✓")
	return db
}

// ConnectDB เปิด connection ด้วย GORM แล้ว AutoMigrate schema ให้ทันที
// schema มาจาก struct tag ใน entity/ ล้วนๆ (ไม่มีไฟล์ .sql แล้ว)
//
// data flow: รับ dbURL (มาจาก config) → เปิด pool → AutoMigrate ทุกตาราง
// → เพิ่ม FK ที่ AutoMigrate ไม่สร้างให้ → คืน *gorm.DB ให้ทุก layer ใช้ query
// ถ้าขั้นไหนพังจะ log.Fatal ทันที (fail fast) — server ไม่ควรขึ้นถ้า schema ยังไม่พร้อม
//
// ลำดับของ AutoMigrate สำคัญ: roles/eligible_students ต้องมาก่อน users (users อ้างถึงทั้งคู่)
// namespaces/request_templates ต้องมาก่อน services (services อ้างทั้งคู่)
// namespaces/users/request_templates ต้องมาก่อน ai_review_requests (อ้างทั้งสาม เหมือน services)
// namespaces/users/eligible_students ต้องมาก่อน namespace_invites (อ้างทั้งสาม)
// ipc_monitors ต้องมาก่อน user_containers (user_containers อ้าง ipc_id)
//
// ใช้กับ cmd/server และ cmd/seed เท่านั้น — เครื่องมืออื่นที่แค่ "อ่าน" DB (เช่น cmd/export-rbac)
// ให้ใช้ ConnectDBReadOnly แทน จะได้ไม่มี DDL (ALTER TABLE/ADD CONSTRAINT) แอบยิงทุกครั้งที่รัน
func ConnectDB(dbURL string) *gorm.DB {
	db := openDB(dbURL)

	if err := db.AutoMigrate(
		&entity.Role{},
		&entity.EligibleStudent{},
		&entity.User{},
		&entity.PasswordResetToken{}, // ต้องมาหลัง users (อ้าง user_id)
		&entity.OTPChallenge{},       // ต้องมาหลัง users (อ้าง user_id)
		&entity.TrustedDevice{},      // ต้องมาหลัง users (อ้าง user_id)
		&entity.Namespace{},
		&entity.NamespaceInvite{}, // ต้องมาหลัง namespaces/users/eligible_students (อ้างทั้งสาม)
		&entity.RequestTemplate{},
		&entity.Service{},
		&entity.AIReviewRequest{}, // ต้องมาหลัง namespaces/users/request_templates (อ้างทั้งสาม)
		&entity.IPCMonitor{},
		&entity.UserContainer{},
		&entity.Request{},
		&entity.UserAlert{},
		&entity.SystemAlert{},
		&entity.AuditLog{},
		&entity.NodeTelemetry{},
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

// ConnectDBReadOnly เปิด connection เฉยๆ — ไม่ AutoMigrate ไม่แตะ FK
// สำหรับเครื่องมือที่อ่าน DB อย่างเดียวและอาจถูกรันบ่อย/อัตโนมัติ (เช่น cmd/export-rbac ที่ตั้งใจ
// ให้รันซ้ำได้เรื่อยๆ ต่อ cron/CI) — ไม่ควรมีผลข้างเคียงเป็น DDL ทุกครั้งที่รัน ต่างจาก ConnectDB
// ที่ตั้งใจให้ migrate ทุกครั้งที่ server/seed start
//
// สมมติว่า schema พร้อมอยู่แล้ว (ผ่าน cmd/server หรือ cmd/seed มาก่อนหน้านี้) ถ้า schema ยังไม่ถูก
// สร้าง query จะ error ธรรมดา ไม่ได้ silently พังแบบเงียบๆ
func ConnectDBReadOnly(dbURL string) *gorm.DB {
	return openDB(dbURL)
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
			// ลบ namespace → ใบเสร็จของ AI review request ที่เคยยื่นในนั้นหายตามไปด้วย (แค่ receipt ไม่ใช่ประวัติที่ต้องเก็บถาวร)
			name: "fk_ai_review_requests_namespace_id",
			ddl: `ALTER TABLE ai_review_requests ADD CONSTRAINT fk_ai_review_requests_namespace_id
			      FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON DELETE CASCADE`,
		},
		{
			name: "fk_ai_review_requests_created_by",
			ddl: `ALTER TABLE ai_review_requests ADD CONSTRAINT fk_ai_review_requests_created_by
			      FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// เหตุผลเดียวกับ fk_services_request_template_id — snapshot cpu/ram ไว้แล้ว ลบ template ไม่ต้องลบใบเสร็จตาม
			name: "fk_ai_review_requests_request_template_id",
			ddl: `ALTER TABLE ai_review_requests ADD CONSTRAINT fk_ai_review_requests_request_template_id
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
			// SET NULL ไม่ใช่ CASCADE โดยตั้งใจ: ลบ service ทิ้งแล้ว "ประวัติว่ามันเคยพัง" ยังมีค่าอยู่
			// (ชื่อ service เก็บไว้ใน source_name อยู่แล้ว อ่านย้อนหลังได้)
			// แต่ service_id ต้องกลายเป็น NULL ไม่งั้นหน้าเว็บจะยังโชว์ปุ่ม "ดู log" ที่กดแล้วพา
			// ไปหา service ที่ไม่มีอยู่แล้ว — frontend เช็ค service_id !== null อยู่แล้ว ปุ่มจะหายเอง
			name: "fk_user_alerts_service_id",
			ddl: `ALTER TABLE user_alerts ADD CONSTRAINT fk_user_alerts_service_id
			      FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE SET NULL`,
		},
		{
			name: "fk_requests_user_id",
			ddl: `ALTER TABLE requests ADD CONSTRAINT fk_requests_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// ลบ user → token รีเซ็ตรหัสผ่านของคนนั้นหายตามไปด้วย
			name: "fk_password_reset_tokens_user_id",
			ddl: `ALTER TABLE password_reset_tokens ADD CONSTRAINT fk_password_reset_tokens_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// ลบ user → ใบยืนยัน OTP ที่ค้างอยู่ของคนนั้นหายตามไปด้วย
			name: "fk_otp_challenges_user_id",
			ddl: `ALTER TABLE otp_challenges ADD CONSTRAINT fk_otp_challenges_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// ลบ user → เครื่องที่เคยเชื่อใจของคนนั้นหายตามไปด้วย
			name: "fk_trusted_devices_user_id",
			ddl: `ALTER TABLE trusted_devices ADD CONSTRAINT fk_trusted_devices_user_id
			      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		},
		{
			// ลบ namespace → คำเชิญที่เคยส่งจากกลุ่มนั้นไม่มีความหมายแล้ว หายตามไปด้วย
			name: "fk_namespace_invites_namespace_id",
			ddl: `ALTER TABLE namespace_invites ADD CONSTRAINT fk_namespace_invites_namespace_id
			      FOREIGN KEY (namespace_id) REFERENCES namespaces(id) ON DELETE CASCADE`,
		},
		{
			// เหตุผลเดียวกับ fk_users_student_id — เชิญได้เฉพาะรหัส นศ. ที่อยู่ในรายชื่อจริง กัน invite ไปหาชื่อมั่ว
			name: "fk_namespace_invites_student_id",
			ddl: `ALTER TABLE namespace_invites ADD CONSTRAINT fk_namespace_invites_student_id
			      FOREIGN KEY (invited_student_id) REFERENCES eligible_students(student_id)`,
		},
		{
			// ลบ user (ผู้เชิญ) → คำเชิญที่เขาเคยส่งหายตามไปด้วย
			name: "fk_namespace_invites_invited_by",
			ddl: `ALTER TABLE namespace_invites ADD CONSTRAINT fk_namespace_invites_invited_by
			      FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE CASCADE`,
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
