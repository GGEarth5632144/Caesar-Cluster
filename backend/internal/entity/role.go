package entity

// ชื่อ role ที่ระบบรู้จัก (seed ใส่ลงตาราง roles ตอน setup)
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Role = ตาราง roles — แยกลำดับการใช้งานระหว่าง user / admin ออกมาเป็นตารางตาม ERD
// (เดิมเก็บเป็น column varchar + CHECK ใน users ตรงๆ)
// ข้อมูลไหลเข้า: cmd/seed ใส่ role ตั้งต้น (user, admin) ครั้งเดียวตอน setup
// ข้อมูลไหลออก: AuthController.Register อ่าน id ของ role "user" ไปผูกกับ user ใหม่,
// AuthController.Login อ่าน name ไปใส่ใน JWT claim "role" ให้ middleware ใช้ต่อ
type Role struct {
	ID   int    `gorm:"column:id;type:serial;primaryKey" json:"id"`
	Name string `gorm:"column:name;type:varchar(20);unique;not null" json:"name"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "roles"
func (Role) TableName() string { return "roles" }
