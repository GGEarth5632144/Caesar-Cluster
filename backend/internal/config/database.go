package config

import (
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/entity"
)

// openDB เปิด connection pool ด้วย GORM เฉยๆ (ยังไม่ migrate) — ใช้ร่วมกันทั้ง ConnectDB
// (server/seed ต้อง migrate) และ ConnectDBReadOnly (เครื่องมือที่อ่านอย่างเดียว ไม่ควร migrate)
func openDB(dbURL string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: gormLogger()})
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	log.Println("database connected ✓")
	return db
}

// gormLogger = logger.Default แต่ไม่นับ "หาแถวไม่เจอ" เป็นเรื่องผิดปกติ
//
// ระบบนี้ใช้ First() + ErrRecordNotFound เป็นทางเดินปกติหลายที่ (เช็คบัญชีซ้ำ, หาใบยืนยันล่าสุด,
// หา user ตอนล็อกอิน) ค่า default พ่น warning ทุกครั้งจน log เต็มไปด้วย "record not found"
// ส่วนกรณีที่หาไม่เจอแล้วผิดปกติจริง handler log เองอยู่แล้วพร้อมบริบท
func gormLogger() logger.Interface {
	return logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})
}

// ConnectDB เปิด connection แล้ว AutoMigrate schema ทันที (schema มาจาก struct tag ใน entity/ ล้วนๆ)
// ขั้นไหนพัง log.Fatal ทันที — server ไม่ควรขึ้นถ้า schema ยังไม่พร้อม
//
// ลำดับ AutoMigrate สำคัญ: ตารางที่ถูกอ้างถึงต้องมาก่อนตารางที่อ้าง (ดูหมายเหตุท้ายแต่ละบรรทัด)
// ใช้กับ cmd/server และ cmd/seed เท่านั้น เครื่องมือที่แค่อ่านให้ใช้ ConnectDBReadOnly
func ConnectDB(dbURL string) *gorm.DB {
	db := openDB(dbURL)

	// ต้องถามก่อน AutoMigrate — หลังจากนี้คอลัมน์จะมีเสมอ แล้วแยกไม่ออกว่า "DB เก่ากว่าฟีเจอร์นี้"
	// (ต้อง backfill) หรือ "แค่มีคนที่ยังไม่กดยืนยัน" (ห้ามแตะ)
	predatesEmailVerification := !columnExists(db, "users", "gmail_verified_at")

	if err := db.AutoMigrate(
		&entity.Role{},
		&entity.EligibleStudent{},
		&entity.User{},
		&entity.PasswordResetToken{}, // ต้องมาหลัง users (อ้าง user_id)
		&entity.EmailVerification{},  // ต้องมาหลัง users (อ้าง user_id)
		&entity.EmailDelivery{},      // ต้องมาหลัง users (อ้าง user_id)
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

	if predatesEmailVerification {
		backfillGmailVerified(db)
	}
	dropRetiredOTPTables(db)

	// ไม่ประกาศ relation ให้ GORM จัดการ FK เอง เพราะเคยเจอว่ามันสร้าง sequence ผิดให้ column ที่เป็น FK
	// (เข้าใจผิดว่าเป็น auto-increment) เลยมาเพิ่ม FK เองด้วย raw SQL — idempotent รันซ้ำได้ทุกครั้งที่ start
	if err := addForeignKeys(db); err != nil {
		log.Fatalf("add foreign keys failed: %v", err)
	}
	log.Println("foreign keys ensured ✓")

	return db
}

// ConnectDBReadOnly เปิด connection เฉยๆ ไม่ AutoMigrate ไม่แตะ FK — สำหรับเครื่องมือที่อ่าน
// อย่างเดียวและถูกรันบ่อย (เช่น cmd/export-rbac) จะได้ไม่มี DDL แอบยิงทุกครั้งที่รัน
func ConnectDBReadOnly(dbURL string) *gorm.DB {
	return openDB(dbURL)
}

// addForeignKeys เพิ่ม FK ทั้งหมดแบบ idempotent (เช็ค pg_constraint ก่อนค่อย ALTER)
//
// FK ที่สำคัญที่สุดคือ users.student_id → eligible_students.student_id — บังคับกฎ "สมัครได้เฉพาะ
// นศ. ที่อยู่ในรายชื่อ" ที่ระดับ DB ต่อให้โค้ดลืมเช็คก็ยัง insert ไม่ผ่าน
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
			// SET NULL ไม่ใช่ CASCADE: ลบ service แล้ว "ประวัติว่ามันเคยพัง" ยังมีค่า (ชื่ออยู่ใน source_name)
			// แต่ service_id ต้องเป็น NULL ไม่งั้นหน้าเว็บโชว์ปุ่ม "ดู log" ที่กดแล้วพาไปหา service ที่ไม่มีแล้ว
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
			// ลบ user → ลิงก์ยืนยันอีเมลที่ค้างอยู่ของคนนั้นหายตามไปด้วย
			name: "fk_email_verifications_user_id",
			ddl: `ALTER TABLE email_verifications ADD CONSTRAINT fk_email_verifications_user_id
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
		// audit_logs / system_alerts / request_templates ไม่มี FK โดยตั้งใจ (ดูเหตุผลในไฟล์ entity ของแต่ละตัว)
		//
		// email_deliveries ก็ไม่มี และห้ามใส่: Register ส่งอีเมลอยู่ข้างใน transaction ที่เพิ่ง INSERT users
		// ถ้ามี FK การเขียน email_deliveries (คนละ connection) จะรอ transaction นั้น commit
		// ส่วน transaction ก็รอผลส่งเมล — ล็อกกันตายทุกครั้งที่มีคนสมัคร
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

// columnExists ถาม Postgres ว่าตาราง/คอลัมน์นี้มีอยู่จริงไหม
// ใช้แยก "DB ที่สร้างใหม่/เก่ากว่าฟีเจอร์" ออกจาก "DB ที่ migrate มาแล้ว" ก่อนตัดสินใจ backfill
// query พังก็ตอบ false ไว้ก่อน — ผลคือถือว่าต้อง backfill ซึ่งเป็นคำสั่งที่รันซ้ำแล้วไม่เสียหาย
func columnExists(db *gorm.DB, table, column string) bool {
	var exists bool
	err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?
	)`, table, column).Scan(&exists).Error
	if err != nil {
		log.Printf("เช็คคอลัมน์ %s.%s ไม่สำเร็จ: %v", table, column, err)
		return false
	}
	return exists
}

// backfillGmailVerified ทำเครื่องหมายว่าบัญชีที่มีอยู่ก่อนฟีเจอร์นี้ "ยืนยันแล้ว" — คอลัมน์ใหม่
// เกิดมาเป็น NULL ทั้งตาราง ปล่อยไว้เท่ากับล็อกนักศึกษาทุกคนออกจากระบบพร้อมกัน
//
// รันเฉพาะรอบที่ AutoMigrate เพิ่งเพิ่มคอลัมน์ (ดู predatesEmailVerification) ถ้ารันทุกครั้งที่
// start จะไปยืนยันให้คนที่ยังไม่กดลิงก์ด้วย = ฟีเจอร์นี้ไร้ผลทันที
func backfillGmailVerified(db *gorm.DB) {
	result := db.Exec(`UPDATE users SET gmail_verified_at = created_at WHERE gmail_verified_at IS NULL`)
	if result.Error != nil {
		log.Fatalf("backfill gmail_verified_at failed: %v", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Printf("ทำเครื่องหมาย 'ยืนยันอีเมลแล้ว' ให้บัญชีเดิม %d รายการ (บัญชีที่มีอยู่ก่อนฟีเจอร์ยืนยันอีเมล) ✓",
			result.RowsAffected)
	}
}

// dropRetiredOTPTables ลบตาราง otp_challenges/trusted_devices ที่เลิกใช้แล้วทิ้ง
// ลบได้เพราะทั้งคู่เก็บแต่ของชั่วคราวที่ไม่มีข้อมูลผู้ใช้ และถ้าปล่อยไว้ FK ที่ยังผูกกับ users
// จะกลายเป็นกับดักตอนลบบัญชีในอนาคต
func dropRetiredOTPTables(db *gorm.DB) {
	for _, table := range []string{"otp_challenges", "trusted_devices"} {
		if !columnExists(db, table, "user_id") {
			continue // ไม่มีตารางนี้อยู่แล้ว (DB ที่สร้างใหม่) ไม่ต้องทำอะไร
		}
		if err := db.Exec(`DROP TABLE IF EXISTS ` + table).Error; err != nil {
			log.Fatalf("drop table %s failed: %v", table, err)
		}
		log.Printf("ลบตาราง %s ที่เลิกใช้แล้ว (ระบบ OTP ถูกแทนด้วยลิงก์ยืนยันอีเมล) ✓", table)
	}
}
