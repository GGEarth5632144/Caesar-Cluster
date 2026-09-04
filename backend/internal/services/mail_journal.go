package services

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
	"backend/internal/mailer"
)

// สั้นกว่า smtpTimeout มากโดยตั้งใจ — ตรงนี้คือ INSERT แถวเดียวที่ไม่มีใครแย่ง lock
// ถ้าช้ากว่านี้แปลว่า DB มีปัญหาอยู่แล้ว ซึ่งไม่ควรถ่วง response ของคนที่กำลังสมัคร
const journalTimeout = 5 * time.Second

// MailJournal บันทึกผลการส่งอีเมลลงตาราง email_deliveries (implement mailer.Recorder)
// ตอบคำถามที่ log ใน stdout ตอบไม่ได้: ส่งไปจริงไหม เมื่อไร พังตรงไหน — ดู entity.EmailDelivery
type MailJournal struct {
	db *gorm.DB
}

// NewMailJournal ประกอบตัวบันทึก — ถูกเรียกจาก controller.NewAuthController ตอนสร้าง mailer
func NewMailJournal(db *gorm.DB) *MailJournal {
	return &MailJournal{db: db}
}

// RecordEmail เขียนผลการส่งหนึ่งฉบับลง DB — ไม่คืน error ตามสัญญาของ mailer.Recorder
//
// ตัดสายจาก ctx ของ request ด้วย WithoutCancel: เมลส่งออกไปแล้วจริง ถ้าผู้ใช้ปิดแท็บระหว่างรอ
// แล้วปล่อยให้บันทึกล้มตาม ตารางจะขาดแถวเฉพาะเคสที่อยากรู้ที่สุดว่าเกิดอะไรขึ้น
func (j *MailJournal) RecordEmail(ctx context.Context, d mailer.Delivery) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), journalTimeout)
	defer cancel()

	row := entity.EmailDelivery{
		UserID:     d.UserID,
		ToEmail:    d.ToEmail,
		Purpose:    d.Purpose,
		MessageID:  d.MessageID,
		Status:     entity.EmailStatusSent,
		DurationMS: int(d.Duration.Milliseconds()),
	}
	if d.Err != nil {
		row.Status = entity.EmailStatusFailed
		row.Error = d.Err.Error()
	}

	if err := j.db.WithContext(writeCtx).Create(&row).Error; err != nil {
		// บันทึกไม่ลงก็ยังต้องเหลือร่องรอยไว้ที่ log อย่างน้อยหนึ่งที่
		log.Printf("mail-journal: บันทึกผลการส่งเมลถึง %s (%s) ไม่สำเร็จ: %v", d.ToEmail, d.Purpose, err)
	}
	if d.Err != nil {
		log.Printf("mail-journal: ส่งเมล %s ถึง %s ไม่สำเร็จ (message-id=%s): %v",
			d.Purpose, d.ToEmail, d.MessageID, d.Err)
	}
}
