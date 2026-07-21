package entity

import "time"

// User = ตาราง users — บัญชีผู้ใช้ (นักศึกษา + admin)
//
// ข้อมูลไหลเข้า: AuthController.Register (ต้องผ่านการเช็ค eligible_students ก่อน) / cmd/seed สร้าง admin
// ข้อมูลไหลออก: AuthController.Login อ่านมาตรวจรหัสผ่านแล้วออก JWT, Password ถูกซ่อน (json:"-") ไม่ส่งออก API
//
// NamespaceID = space ที่ผู้ใช้สังกัด — ตามที่ตกลงกันไว้ว่า "1 คน = 1 space" เลยเก็บเป็น column ตรงนี้เลย
// ไม่ต้องมีตารางเชื่อม (ตรงกับ user.space_id ใน ERD) เป็น pointer เพราะ user ที่เพิ่งสมัครยังไม่มี space (NULL)
// member_count ของกลุ่มไม่ได้เก็บซ้ำไว้ที่ไหน — นับจาก COUNT(users WHERE namespace_id = ?) เอา กันค่าเพี้ยน
//
// หมายเหตุ student_id ใช้ tag `unique` (ไม่ใช่ `uniqueIndex`) โดยตั้งใจ:
// ถ้าใช้ uniqueIndex, AutoMigrate จะเข้าใจว่า column ไม่ควร unique แล้วสั่ง DROP CONSTRAINT
// uni_users_student_id (ชื่อที่ GORM เดาเอง) ซึ่งไม่มีจริง → migrate พัง
type User struct {
	ID          int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	StudentID   string    `gorm:"column:student_id;type:varchar(20);unique;not null" json:"student_id"`
	RoleID      int       `gorm:"column:role_id;type:integer;not null;index:idx_users_role" json:"role_id"`
	RealName    string    `gorm:"column:real_name;type:varchar(100);not null" json:"real_name"`
	NickName    string    `gorm:"column:nick_name;type:varchar(50)" json:"nick_name"`
	NamespaceID *int      `gorm:"column:namespace_id;type:integer;index:idx_users_namespace" json:"namespace_id"`
	Password    string    `gorm:"column:password;type:varchar(255);not null" json:"-"`
	Gmail 		string 	  `gorm:"column:gmail;type:varchar(100);unique;not null" json:"gmail"`
	// EntryYear = ปีที่เข้าศึกษา (พ.ศ.) แกะจาก prefix ของ StudentID ตอนสมัครครั้งเดียว (entity.EntryYearFromStudentID)
	// เก็บไว้ได้เพราะเป็นข้อเท็จจริงที่ไม่เปลี่ยน — ต่างจาก "ชั้นปี" ที่ต้องคำนวณสดทุกครั้ง (ดู entity.YearLevel)
	EntryYear	int 	  `gorm:"column:year;type:integer;not null;default:0" json:"year"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
	CPUlimit	int 	  `gorm:"column:cpu_limit;type:integer;not null;default:0" json:"cpu_limit"`
	Ramlimit    int 	  `gorm:"column:ram_limit;type:integer;not null;default:0" json:"ram_limit"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "users"
func (User) TableName() string { return "users" }
