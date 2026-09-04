package services

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

var ErrAlertNotFound = errors.New("ไม่พบแจ้งเตือนนี้")

// defaultAlertMergeWindow = ช่วงเวลาที่ถือว่า error เดิมยัง "เป็นเรื่องเดียวกัน" อยู่
//
// ภายในหน้าต่างนี้ error ที่ fingerprint ตรงกันจะไปบวก count ของแถวเดิมแทนการสร้างแถวใหม่
// พ้นหน้าต่างแล้วค่อยนับเป็นแจ้งเตือนใหม่ — ปัญหาที่เงียบไปครึ่งวันแล้วกลับมา ควรเด้งเตือนอีกครั้ง
// ไม่ใช่ไปบวกเลขเงียบๆ ในแถวที่ผู้ใช้กดอ่านไปแล้ว
const defaultAlertMergeWindow = 6 * time.Hour

// maxAlertsPerUser = จำนวนแถวสูงสุดที่เก็บไว้ต่อ user — เกินกว่านี้ตัดตัวเก่าที่อ่านแล้วทิ้ง
// กันตาราง user_alerts โตไม่มีที่สิ้นสุดจาก service ที่พังค้างไว้เป็นเดือนโดยไม่มีใครมาลบ
const maxAlertsPerUser = 200

// RaiseParams = ข้อมูลของแจ้งเตือน 1 เรื่อง ที่จะยิงถึง user หลายคนพร้อมกัน
//
// UserIDs เป็นหลายคนเพราะ service เป็นของ "ทั้ง space" — สมาชิกทุกคนในกลุ่มควรรู้ว่า
// service ของกลุ่มพ่น error เหมือนกันหมด (สอดคล้องกับที่ ServiceManager.ListByNamespace
// ให้ทุกคนในกลุ่มเห็น service ชุดเดียวกัน)
type RaiseParams struct {
	UserIDs     []int
	Severity    string // entity.SeverityCritical | SeverityWarning | SeverityInfo
	Title       string
	Message     string
	SourceType  string // entity.AlertSourceServiceLog | AlertSourceSystem
	SourceName  string
	ServiceID   *int
	Fingerprint string // ตัวชี้ว่า "เรื่องเดียวกัน" — ว่าง = ไม่ยุบ สร้างแถวใหม่เสมอ
	Count       int    // จำนวนครั้งที่เจอในรอบนี้ (0/1 = ครั้งเดียว)
	MergeWindow time.Duration
}

// AlertManager = business logic ของแจ้งเตือนรายคน: สร้าง (พร้อมยุบเรื่องซ้ำ), อ่าน, mark read, ลบ
//
// ไม่รู้จัก k8s หรือ HTTP เลย — LogAlertScanner เป็นคนป้อนข้อมูลเข้า, AlertController เป็นคนอ่านออก
type AlertManager struct {
	db *gorm.DB
}

// NewAlertManager ประกอบ manager — ถูกเรียกจาก main ตอน start
func NewAlertManager(db *gorm.DB) *AlertManager { return &AlertManager{db: db} }

// Raise บันทึกแจ้งเตือนให้ user ทุกคนใน p.UserIDs
//
// data flow: LogAlertScanner เจอ error ในรอบสแกน → เรียก Raise ครั้งเดียวต่อ 1 fingerprint
// → ต่อ user 1 คน: ลอง UPDATE แถวเดิมที่ fingerprint ตรงและยังอยู่ในหน้าต่างเวลาก่อน
// → ไม่มีแถวไหนโดน (RowsAffected = 0) ค่อย INSERT แถวใหม่
//
// ตั้งใจไม่รีเซ็ต is_read ตอนยุบ: ถ้ารีเซ็ต ตัวเลขบน Sidebar จะกดให้หายไม่ได้เลยตราบใดที่
// service ยังพ่น error เดิม (นับใหม่ทุกรอบสแกน) กลายเป็นตัวเลขแดงที่ผู้ใช้ทำอะไรไม่ได้จนเลิกสนใจ
// — ปล่อยให้ count เดินเงียบๆ แล้วไปเด้งใหม่เมื่อพ้นหน้าต่างเวลาแทน
func (m *AlertManager) Raise(ctx context.Context, p RaiseParams) error {
	if len(p.UserIDs) == 0 {
		return nil
	}
	if p.Count < 1 {
		p.Count = 1
	}
	if p.MergeWindow <= 0 {
		p.MergeWindow = defaultAlertMergeWindow
	}
	if p.Severity == "" {
		p.Severity = entity.SeverityInfo
	}
	if p.SourceType == "" {
		p.SourceType = entity.AlertSourceSystem
	}

	now := time.Now()
	cutoff := now.Add(-p.MergeWindow)

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, uid := range p.UserIDs {
			if p.Fingerprint != "" {
				res := tx.Model(&entity.UserAlert{}).
					Where("user_id = ? AND fingerprint = ? AND last_seen_at > ?", uid, p.Fingerprint, cutoff).
					Updates(map[string]any{
						"count":        gorm.Expr("count + ?", p.Count),
						"last_seen_at": now,
						"message":      p.Message, // เก็บตัวอย่างล่าสุดไว้ ผู้ใช้จะได้เห็นบรรทัดที่เพิ่งเกิด
					})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					continue
				}
			}

			alert := entity.UserAlert{
				UserID:      uid,
				Severity:    p.Severity,
				Title:       p.Title,
				Message:     p.Message,
				SourceType:  p.SourceType,
				SourceName:  p.SourceName,
				ServiceID:   p.ServiceID,
				Fingerprint: p.Fingerprint,
				Count:       p.Count,
				CreatedAt:   now,
				LastSeenAt:  now,
			}
			if err := tx.Create(&alert).Error; err != nil {
				return err
			}
			if err := trimAlerts(tx, uid); err != nil {
				return err
			}
		}
		return nil
	})
}

// trimAlerts ตัดแจ้งเตือนเก่าทิ้งเมื่อ user คนหนึ่งมีเกิน maxAlertsPerUser แถว
//
// ลบจากตัวที่เก่าที่สุดก่อน แต่ข้ามตัวที่ยังไม่ได้อ่าน (ไม่ลบของที่ผู้ใช้ยังไม่เคยเห็น
// ไม่งั้นตัวเลขบน Sidebar จะลดลงเองโดยผู้ใช้ไม่ได้ทำอะไร ซึ่งอธิบายไม่ได้)
func trimAlerts(tx *gorm.DB, userID int) error {
	var total int64
	if err := tx.Model(&entity.UserAlert{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return err
	}
	if total <= maxAlertsPerUser {
		return nil
	}
	return tx.Where(
		"id IN (?)",
		tx.Model(&entity.UserAlert{}).
			Select("id").
			Where("user_id = ? AND is_read = true", userID).
			Order("last_seen_at ASC").
			Limit(int(total-maxAlertsPerUser)),
	).Delete(&entity.UserAlert{}).Error
}

// ListForUser คืนแจ้งเตือนของ user เรียงใหม่→เก่าตามเวลาที่เจอครั้งล่าสุด
// data flow: AlertController.List → ที่นี่ → หน้า Alerts ของ frontend
//
// limit ที่เกินเพดานถูก "หนีบลงมาที่เพดาน" ไม่ใช่ตกกลับไปเป็น 50 — ไม่งั้นการขอ 300
// จะได้ของน้อยกว่าการขอ 100 ซึ่งไม่มีใครเดาถูก
func (m *AlertManager) ListForUser(ctx context.Context, userID, limit int, onlyUnread bool) ([]entity.UserAlert, error) {
	switch {
	case limit <= 0:
		limit = 50
	case limit > maxAlertsPerUser:
		limit = maxAlertsPerUser
	}
	q := m.db.WithContext(ctx).Where("user_id = ?", userID)
	if onlyUnread {
		q = q.Where("is_read = false")
	}
	var list []entity.UserAlert
	err := q.Order("last_seen_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// UnreadCount นับแจ้งเตือนที่ยังไม่ได้อ่าน — เป็นตัวเลขในวงกลมแดงบน Sidebar โดยตรง
// นับเป็น "จำนวนเรื่อง" ไม่ใช่ผลรวมของ count เพราะผู้ใช้กดเข้าไปแล้วจะเห็นกี่แถว ตัวเลขก็ควรเป็นเท่านั้น
func (m *AlertManager) UnreadCount(ctx context.Context, userID int) (int64, error) {
	var n int64
	err := m.db.WithContext(ctx).Model(&entity.UserAlert{}).
		Where("user_id = ? AND is_read = false", userID).Count(&n).Error
	return n, err
}

// MarkRead ทำเครื่องหมายว่าอ่านแล้ว — ids ว่าง = ทั้งหมดของ user คนนี้
// เงื่อนไข user_id เสมอ กันไม่ให้ mark ของคนอื่นด้วยการเดา id
func (m *AlertManager) MarkRead(ctx context.Context, userID int, ids []int) (int64, error) {
	q := m.db.WithContext(ctx).Model(&entity.UserAlert{}).Where("user_id = ? AND is_read = false", userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	res := q.Update("is_read", true)
	return res.RowsAffected, res.Error
}

// Delete ลบแจ้งเตือนทีละอัน (ปุ่มถังขยะบนการ์ดในหน้า Alerts)
func (m *AlertManager) Delete(ctx context.Context, userID, id int) error {
	res := m.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).Delete(&entity.UserAlert{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAlertNotFound
	}
	return nil
}

// DeleteRead ล้างแจ้งเตือนที่อ่านแล้วทิ้งทั้งหมด (ปุ่ม "ล้างที่อ่านแล้ว")
// ไม่แตะตัวที่ยังไม่อ่าน เพื่อไม่ให้เผลอลบของที่ยังไม่เคยเห็นด้วยการกดปุ่มเดียว
func (m *AlertManager) DeleteRead(ctx context.Context, userID int) (int64, error) {
	res := m.db.WithContext(ctx).
		Where("user_id = ? AND is_read = true", userID).Delete(&entity.UserAlert{})
	return res.RowsAffected, res.Error
}
