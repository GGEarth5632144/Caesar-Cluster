package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/utils"
)

// ต้องเว้นกี่วินาทีถึงจะขอลิงก์ใบใหม่ได้ — คุมที่ระดับบัญชี ไม่ใช่ IP เพราะนักศึกษาทั้งแล็บ
// ออกเน็ตผ่าน IP เดียวกัน สิ่งที่กันคือการเอา endpoint นี้ไปถล่มกล่องอีเมลของคนอื่น
const verificationResendCooldown = 60 * time.Second

// sendVerificationLink คืนตัวนี้เมื่อส่งอีเมลไม่ออก — Register ต้องแยกให้ได้ว่า rollback
// เพราะเมลไม่ออก (ลองใหม่ได้) ไม่ใช่เพราะข้อมูลชน (ลองใหม่ก็ได้ผลเดิม)
var errVerificationMailFailed = errors.New("ส่งอีเมลยืนยันไม่สำเร็จ")

// sendVerificationLink คืนตัวนี้เมื่อเพิ่งส่งลิงก์ไปยังไม่ครบ cooldown
var errVerificationResendTooSoon = errors.New("เพิ่งส่งลิงก์ยืนยันไปเมื่อสักครู่")

// ตอบกลับ /resend-verification เสมอไม่ว่าอีเมลนั้นจะมีบัญชีอยู่จริงหรือไม่
// เหตุผลเดียวกับ genericForgotMsg: ไม่เปิดช่องให้ไล่เดาว่าอีเมลไหนมีบัญชีในระบบ
const genericResendMsg = "ถ้ามีบัญชีที่ใช้อีเมลนี้และยังไม่ได้ยืนยัน เราได้ส่งลิงก์ยืนยันใบใหม่ไปให้แล้ว " +
	"กรุณาตรวจสอบกล่องอีเมล รวมทั้งเมลขยะ (Spam)"

// VerifyEmail เปิดใช้งานบัญชีจาก token ในลิงก์อีเมล แล้วออก JWT ให้เข้าใช้งานต่อได้ทันที
// (กดลิงก์ในกล่องอีเมลตัวเองพิสูจน์ตัวตนครบแล้ว — หลักฐานชุดเดียวกับลิงก์รีเซ็ตรหัสผ่าน)
//
// แลกกับการที่ลิงก์กลายเป็นกุญแจเข้าบัญชี จึงต้องคุม used_at และ expires_at ให้แน่น
//
// ไม่กรอง used_at/expires_at ตั้งแต่ใน query เพราะทุกกรณีที่ไม่ผ่านจะยุบเหลือ "ไม่พบ" เหมือนกันหมด
// ซึ่งผิดกับสองเคสที่เกิดบ่อยสุด: กดลิงก์ซ้ำ (ต้องบอกว่ายืนยันแล้ว) และลิงก์หมดอายุ (ต้องให้ขอใบใหม่)
func (h *AuthController) VerifyEmail(c *gin.Context) {
	var req dto.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	var row entity.EmailVerification
	if err := db.Where("token_hash = ?", hashToken(req.Token)).First(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("verify-email: query token ไม่สำเร็จ: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
		utils.Error(c, http.StatusBadRequest, "INVALID_TOKEN",
			"ลิงก์ยืนยันไม่ถูกต้อง กรุณาเปิดลิงก์ล่าสุดจากอีเมลอีกครั้ง")
		return
	}

	var user entity.User
	if err := db.First(&user, row.UserID).Error; err != nil {
		log.Printf("verify-email: หา user %d ของ token ไม่เจอ: %v", row.UserID, err)
		utils.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "ลิงก์ยืนยันไม่ถูกต้องหรือหมดอายุแล้ว")
		return
	}

	// ลิงก์ทำหน้าที่ของมันไปแล้ว — ตอบว่าสำเร็จ แต่ไม่ออก session ให้ ไม่งั้นลิงก์ที่ค้างในกล่องอีเมล
	// จะกลายเป็นกุญแจที่กดกี่ครั้งก็เข้าบัญชีได้
	if user.GmailVerified() || row.UsedAt != nil {
		utils.OK(c, http.StatusOK, gin.H{"already": true})
		return
	}
	if time.Now().After(row.ExpiresAt) {
		utils.Error(c, http.StatusBadRequest, "TOKEN_EXPIRED",
			"ลิงก์ยืนยันหมดอายุแล้ว กรุณากดขอลิงก์ใหม่")
		return
	}

	var role entity.Role
	if err := db.First(&role, user.RoleID).Error; err != nil {
		log.Printf("verify-email: หา role ของ user %d ไม่เจอ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยืนยันอีเมลไม่สำเร็จ")
		return
	}

	now := dbNow()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&row).Update("used_at", &now).Error; err != nil {
			return err
		}
		return tx.Model(&entity.User{}).Where("id = ?", user.ID).
			Update("gmail_verified_at", &now).Error
	})
	if err != nil {
		log.Printf("verify-email: เปิดใช้งานบัญชี %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยืนยันอีเมลไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
		return
	}

	// remember = false: การกดลิงก์ยืนยันไม่ใช่การแสดงเจตนา "จำฉันไว้"
	payload, err := h.buildSession(&user, role.Name, false)
	if err != nil {
		log.Printf("verify-email: สร้าง token ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง token ไม่สำเร็จ")
		return
	}
	// ติด already ไปทั้งสองทาง หน้าเว็บจะได้ดู field เดียวแล้วรู้ว่าจะพาเข้า dashboard หรือหน้า login
	payload["already"] = false
	utils.OK(c, http.StatusOK, payload)
}

// ResendVerification ส่งลิงก์ยืนยันใบใหม่ให้บัญชีที่ยังไม่ได้ยืนยัน
//
// ตอบ genericResendMsg เหมือนกันทุกกรณี รวมถึงตอนติด cooldown ด้วย — ถ้าตอบ 429 เฉพาะตอนติด
// cooldown จะกลายเป็นเครื่องมือไล่เดาทันที (ยิงอีเมลเดิมสองครั้งติด ได้ 429 = อีเมลนี้มีบัญชีอยู่)
// ไม่เสีย UX เพราะหน้าเว็บนับ cooldown ของตัวเองอยู่แล้ว (ดู ResendVerification.tsx)
func (h *AuthController) ResendVerification(c *gin.Context) {
	var req dto.ResendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	var user entity.User
	err := db.Where("lower(gmail) = ? AND gmail_verified_at IS NULL",
		normalizeGmail(req.Gmail)).First(&user).Error
	switch {
	case err == nil:
		// ส่งไม่สำเร็จก็ยังตอบ generic — บอกไปเท่ากับยืนยันว่ามีบัญชีนี้
		// รายละเอียดอยู่ครบใน log + ตาราง email_deliveries ให้ admin ไล่ดูได้
		if err := h.sendVerificationLink(c.Request.Context(), db, &user); err != nil {
			log.Printf("resend-verification: ส่งลิงก์ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		log.Printf("resend-verification: query user error: %v", err)
	}

	utils.OK(c, http.StatusOK, gin.H{"message": genericResendMsg})
}

// sendVerificationLink ออกลิงก์ใบใหม่แล้วส่งอีเมล — ใช้ร่วมกันทั้ง Register, สมัครซ้ำ และขอลิงก์ใหม่
// รับ db เป็นพารามิเตอร์เพราะ Register เรียกจากใน transaction ถ้าใช้ h.db แถวจะไป rollback ไม่พร้อมกัน
func (h *AuthController) sendVerificationLink(ctx context.Context, db *gorm.DB, user *entity.User) error {
	// cooldown อ่านจากใบล่าสุดใน DB ไม่ใช่ตัวนับในหน่วยความจำ — restart แล้วยังนับต่อ และนับถูกแม้มีหลาย instance
	var latest entity.EmailVerification
	err := db.Where("user_id = ?", user.ID).Order("created_at DESC").First(&latest).Error
	switch {
	case err == nil:
		if time.Since(latest.CreatedAt) < verificationResendCooldown {
			return errVerificationResendTooSoon
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}

	// ใบเก่าที่ยังไม่ถูกใช้ตายทันทีเมื่อออกใบใหม่ ไม่งั้นลิงก์จากหลายฉบับใช้ได้พร้อมกัน
	// ลบทิ้งแทนการ set ให้หมดอายุ เพราะผลเหมือนกันแต่ไม่มีแถวตายค้างให้ต้องมี job มากวาด
	//
	// ใบที่ใช้แล้วไม่ลบ: VerifyEmail ต้องใช้มันตอบว่า "ยืนยันไปแล้ว" ให้คนที่กดลิงก์ซ้ำ
	// เงื่อนไข user_id ยังกัน lock ชนกันด้วย — Register ถือ transaction นี้ค้างตลอดช่วงคุย SMTP (~15 วิ)
	if err := db.Where("user_id = ? AND used_at IS NULL", user.ID).
		Delete(&entity.EmailVerification{}).Error; err != nil {
		return err
	}

	plainToken, err := randomToken()
	if err != nil {
		return err
	}
	row := entity.EmailVerification{
		UserID:    user.ID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: dbNow().Add(time.Duration(h.cfg.VerifyTokenTTLHours) * time.Hour),
	}
	if err := db.Create(&row).Error; err != nil {
		return err
	}

	link := strings.TrimRight(h.cfg.FrontendOrigin, "/") + "/verify-email?token=" + plainToken
	if err := h.mailer.SendVerificationEmail(
		ctx, user.ID, user.Gmail, user.RealName, link, h.cfg.VerifyTokenTTLHours,
	); err != nil {
		// ห่อด้วย sentinel ให้ Register แยกออกว่า rollback เพราะเมลไม่ออก ไม่ใช่เพราะ DB
		// (error ตัวจริงถูกบันทึกไว้ครบใน email_deliveries โดย MailJournal แล้ว)
		return errors.Join(errVerificationMailFailed, err)
	}
	return nil
}

// normalizeGmail ตัดช่องว่างหัวท้าย + ทำเป็นตัวพิมพ์เล็ก ต้องใช้ทุกที่ที่รับอีเมลจากผู้ใช้
// ไม่งั้น "A@x.com" กับ "a@x.com" จะกลายเป็นคนละบัญชี
func normalizeGmail(gmail string) string {
	return strings.ToLower(strings.TrimSpace(gmail))
}

// randomToken สุ่ม token 32 ไบต์เป็น hex สำหรับลิงก์ยืนยันอีเมล
func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
