package entity

import "time"

// สถานะของคำเชิญเข้ากลุ่ม
const (
	InviteStatusPending  = "pending"
	InviteStatusAccepted = "accepted"
	InviteStatusDeclined = "declined"
)

// NamespaceInvite = ตาราง namespace_invites — คำเชิญที่เจ้าของ (contributor) ของ namespace ส่งถึง
// student_id คนใดคนหนึ่งให้เข้ามาเป็นสมาชิกกลุ่ม (แก้ปัญหาที่ Join เดิมต้องรู้ namespace_id ตรงๆ
// ซึ่งเป็นเลขเรียงลำดับที่เดาได้ และใครก็ Join ใครได้โดยเจ้าของไม่ยินยอม)
//
// เก็บเป็น student_id (string) ไม่ใช่ user_id (int) ตั้งใจ เพราะคนที่ถูกเชิญอาจยังไม่ได้ลงทะเบียน
// ในระบบเลยก็ได้ — เชิญล่วงหน้าไว้ก่อนได้ พอ register/login แล้วค่อยเห็นคำเชิญของตัวเอง (ดู InviteManager.Mine)
//
// ข้อมูลไหลเข้า: InviteController.Create (เฉพาะ contributor เชิญได้)
// ข้อมูลไหลออก: InviteController.Mine/Sent อ่านไปโชว์, Accept/Decline อัปเดต status
type NamespaceInvite struct {
	ID               int       `gorm:"column:id;type:serial;primaryKey" json:"id"`
	NamespaceID      int       `gorm:"column:namespace_id;type:integer;not null;index:idx_namespace_invites_namespace" json:"namespace_id"`
	InvitedStudentID string    `gorm:"column:invited_student_id;type:varchar(20);not null;index:idx_namespace_invites_student" json:"invited_student_id"`
	InvitedBy        int       `gorm:"column:invited_by;type:integer;not null" json:"invited_by"`
	Status           string    `gorm:"column:status;type:varchar(10);not null;default:pending;check:status IN ('pending','accepted','declined')" json:"status"`
	CreatedAt        time.Time `gorm:"column:created_at;type:timestamp;not null;default:now()" json:"created_at"`
}

// TableName บอก GORM ให้ map struct นี้กับตาราง "namespace_invites"
func (NamespaceInvite) TableName() string { return "namespace_invites" }
