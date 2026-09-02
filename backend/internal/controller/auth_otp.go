package controller

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/utils"
)

// ค่าคุมพฤติกรรมของ OTP — เป็น const ในโค้ด ไม่ใช่ env โดยตั้งใจ
// ทั้งหมดเป็นการตัดสินใจด้านความปลอดภัย ไม่ใช่ค่าที่ควรต่างกันไปตามเครื่องที่ deploy
// (เปิดให้ตั้งผ่าน .env = เปิดช่องให้ตั้ง trustedDeviceTTL เป็น 10 ปีจนระบบ OTP ไม่เหลือความหมาย)
const (
	// อายุของรหัสหนึ่งชุด — สั้นพอที่รหัสหลุดแล้วใช้ไม่ทัน ยาวพอให้เปิดอีเมลในมือถือทัน
	otpTTL = 10 * time.Minute

	// กรอกผิดได้กี่ครั้งต่อใบก่อนใบตาย — รหัส 6 หลักมีล้านค่า ปล่อยให้เดาไม่จำกัดก็เดาถูกในไม่กี่นาที
	// 5 ครั้งเผื่อคนพิมพ์ผิด/อ่านเลขสลับหลัก แต่โอกาสเดาถูกยังเหลือ 5 ในล้าน
	otpMaxAttempts = 5

	// cooldown + เพดานของปุ่ม "ส่งรหัสใหม่" ต่อหนึ่งใบ (ปกติคนกดครั้งเดียวตอนอีเมลมาช้า)
	otpResendCooldown = 60 * time.Second
	otpMaxResends     = 3

	// ออกใบใหม่ให้ user คนหนึ่งได้กี่ใบต่อช่วงเวลา — กันคนที่รู้รหัสผ่านเหยื่ออยู่แล้ว
	// ล็อกอินซ้ำๆ เพื่อถล่มอีเมลเหยื่อ
	otpMaxChallengesPerWindow = 5
	otpChallengeWindow        = 15 * time.Minute

	// ผ่าน OTP หนึ่งครั้งแล้วเครื่องนั้นข้าม OTP ได้นานแค่ไหน
	// 30 วันให้ตรงกับ "Remember For 30 Days" ที่หน้า login มีอยู่แล้ว จะได้ไม่มีสองช่วงเวลาให้จำ
	trustedDeviceTTL = 30 * 24 * time.Hour

	// อายุรวมของใบหนึ่งใบ นับจากตอนล็อกอิน — ยาวกว่า otpTTL เพราะ "รหัสหมดอายุ" กับ "ใบตาย"
	// คนละเรื่อง (เปิดอีเมลช้าไปควรกดขอรหัสใหม่ได้ ไม่ต้องกรอกรหัสผ่านใหม่) แต่ต้องมีเพดาน
	// ไม่งั้นใบเดียวถูกยืดอายุด้วยการขอรหัสใหม่ไปเรื่อยๆ — บังคับที่ ResendOTP
	otpChallengeMaxLifetime = 30 * time.Minute

	// เก็บใบที่ใช้แล้ว/หมดอายุไว้นานแค่ไหนก่อนกวาดทิ้ง
	// กวาดตอนมีคนล็อกอินแทน background job — ตารางนี้โตช้ามาก ไม่คุ้มกับ goroutine อีกตัว
	otpChallengeRetention = 24 * time.Hour
)

// VerifyOTP เอารหัส 6 หลักจากอีเมลมาแลกเป็น JWT
//
// data flow:
//   - JSON body → bind VerifyOTPRequest (challenge_token + code 6 หลัก)
//   - หา challenge จาก hash ของ token → ต้องยังไม่ถูกใช้ ยังไม่หมดอายุ และยังกรอกผิดไม่ครบเพดาน
//   - เทียบรหัสด้วย bcrypt → ผิดก็บวก attempts แล้วตอบว่าเหลือกี่ครั้ง
//   - ถูก → mark consumed → ออก device token (trusted_devices) → ออก JWT เหมือน Login ปกติ
//
// ตอบ device_token กลับไปให้ client เก็บไว้ ครั้งหน้าแนบมากับ /api/login จะข้าม OTP ได้เลย
func (h *AuthController) VerifyOTP(c *gin.Context) {
	var req dto.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	challenge, err := findLiveChallenge(db, req.ChallengeToken)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_CHALLENGE",
			"คำขอยืนยันตัวตนไม่ถูกต้องหรือหมดอายุแล้ว กรุณาเข้าสู่ระบบใหม่อีกครั้ง")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(challenge.CodeHash), []byte(req.Code)) != nil {
		challenge.Attempts++
		if err := db.Model(challenge).Update("attempts", challenge.Attempts).Error; err != nil {
			log.Printf("verify-otp: อัปเดต attempts ของ challenge %d ไม่สำเร็จ: %v", challenge.ID, err)
		}
		remaining := otpMaxAttempts - challenge.Attempts
		if remaining <= 0 {
			utils.Error(c, http.StatusBadRequest, "INVALID_CHALLENGE",
				"กรอกรหัสผิดเกินจำนวนครั้งที่กำหนด กรุณาเข้าสู่ระบบใหม่อีกครั้ง")
			return
		}
		utils.Error(c, http.StatusBadRequest, "INVALID_CODE",
			fmt.Sprintf("รหัสยืนยันไม่ถูกต้อง เหลือโอกาสอีก %d ครั้ง", remaining))
		return
	}

	now := time.Now()
	if err := db.Model(challenge).Update("consumed_at", &now).Error; err != nil {
		log.Printf("verify-otp: mark consumed ของ challenge %d ไม่สำเร็จ: %v", challenge.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยืนยันตัวตนไม่สำเร็จ")
		return
	}

	var user entity.User
	if err := db.First(&user, challenge.UserID).Error; err != nil {
		log.Printf("verify-otp: หา user %d ไม่เจอ: %v", challenge.UserID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยืนยันตัวตนไม่สำเร็จ")
		return
	}
	var role entity.Role
	if err := db.First(&role, user.RoleID).Error; err != nil {
		log.Printf("verify-otp: หา role ของ user %d ไม่เจอ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยืนยันตัวตนไม่สำเร็จ")
		return
	}

	payload, err := h.buildSession(&user, role.Name, challenge.Remember)
	if err != nil {
		log.Printf("verify-otp: สร้าง token ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง token ไม่สำเร็จ")
		return
	}

	// ออกใบผ่านของเครื่องนี้ — พังก็ไม่ถือว่า login ล้มเหลว แค่ต้องกรอก OTP ใหม่ในครั้งหน้า
	deviceToken, err := issueTrustedDevice(db, user.ID)
	if err != nil {
		log.Printf("verify-otp: ออก trusted device ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
	} else {
		payload["device_token"] = deviceToken
	}

	utils.OK(c, http.StatusOK, payload)
}

// ResendOTP ออกรหัสใหม่ให้ใบเดิมแล้วส่งอีเมลอีกรอบ (กรณีอีเมลไม่มา/ลบทิ้งไปแล้ว)
//
// ใช้ใบเดิมไม่ออกใบใหม่ challenge token ที่ client ถืออยู่จะได้ไม่ต้องเปลี่ยน
// attempts ที่กรอกผิดไปแล้วไม่รีเซ็ตโดยตั้งใจ ไม่งั้นการกดส่งรหัสใหม่จะเป็นช่องให้เดารหัสไม่จำกัด
// (ผิดครบ 5 ครั้ง → กดส่งใหม่ → เดาต่อได้อีก 5 ครั้ง วนไปเรื่อยๆ)
func (h *AuthController) ResendOTP(c *gin.Context) {
	var req dto.ResendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	// ตั้งใจไม่ใช้ findLiveChallenge ตรงนี้ — ตอนกดขอรหัสใหม่ รหัสเดิมมักหมดอายุไปแล้ว
	// ซึ่งเป็นเหตุผลที่กดพอดี ถ้าเช็คด้วยเงื่อนไขเดียวกับตอน verify จะปฏิเสธทุกครั้งที่จำเป็นจริงๆ
	challenge, err := findResendableChallenge(db, req.ChallengeToken)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_CHALLENGE",
			"คำขอยืนยันตัวตนไม่ถูกต้องหรือหมดอายุแล้ว กรุณาเข้าสู่ระบบใหม่อีกครั้ง")
		return
	}

	if wait := otpResendCooldown - time.Since(challenge.LastSentAt); wait > 0 {
		utils.Error(c, http.StatusTooManyRequests, "RESEND_TOO_SOON",
			fmt.Sprintf("เพิ่งส่งรหัสไปเมื่อสักครู่ กรุณารออีก %d วินาที", int(wait.Seconds())+1))
		return
	}
	if challenge.Resends >= otpMaxResends {
		utils.Error(c, http.StatusTooManyRequests, "RESEND_LIMIT",
			"ขอรหัสใหม่ครบจำนวนครั้งที่กำหนดแล้ว กรุณาเข้าสู่ระบบใหม่อีกครั้ง")
		return
	}

	// รหัสใหม่ต้องไม่ยืดใบให้เกิน otpChallengeMaxLifetime — ไม่งั้นการกดขอรหัสใหม่ตอนใบใกล้ตาย
	// จะต่ออายุใบออกไปอีก otpTTL เต็มๆ ทุกครั้ง ทำให้อายุรวมจริงยาวกว่าเพดานที่ประกาศไว้
	now := time.Now()
	expiresAt := now.Add(otpTTL)
	if deadline := challenge.CreatedAt.Add(otpChallengeMaxLifetime); expiresAt.After(deadline) {
		expiresAt = deadline
	}
	// เหลือเวลาไม่พอให้เปิดอีเมลมากรอกทัน — ให้เริ่มล็อกอินใหม่ ดีกว่าส่งรหัสที่ตายก่อนถึงมือ
	if time.Until(expiresAt) < otpResendCooldown {
		utils.Error(c, http.StatusBadRequest, "INVALID_CHALLENGE",
			"คำขอยืนยันตัวตนนี้หมดอายุแล้ว กรุณาเข้าสู่ระบบใหม่อีกครั้ง")
		return
	}

	var user entity.User
	if err := db.First(&user, challenge.UserID).Error; err != nil {
		log.Printf("resend-otp: หา user %d ไม่เจอ: %v", challenge.UserID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ส่งรหัสใหม่ไม่สำเร็จ")
		return
	}

	code, codeHash, err := newOTPCode()
	if err != nil {
		log.Printf("resend-otp: สร้างรหัสใหม่ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ส่งรหัสใหม่ไม่สำเร็จ")
		return
	}

	if err := db.Model(challenge).Updates(map[string]any{
		"code_hash":    codeHash,
		"expires_at":   expiresAt,
		"last_sent_at": now,
		"resends":      challenge.Resends + 1,
	}).Error; err != nil {
		log.Printf("resend-otp: อัปเดต challenge %d ไม่สำเร็จ: %v", challenge.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ส่งรหัสใหม่ไม่สำเร็จ")
		return
	}

	// บอกเวลาที่เหลือจริง ไม่ใช่ otpTTL เต็ม — รหัสรอบท้ายๆ อาจถูกหนีบด้วยเพดานอายุใบ
	if err := h.mailer.SendOTPEmail(
		c.Request.Context(), user.Gmail, user.RealName, code, int(time.Until(expiresAt).Minutes()),
	); err != nil {
		log.Printf("resend-otp: ส่งอีเมลให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusBadGateway, "MAIL_FAILED",
			"ส่งอีเมลไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{
		"message":            "ส่งรหัสยืนยันใหม่ไปที่อีเมลของคุณแล้ว",
		"expires_in_seconds": int(time.Until(expiresAt).Seconds()),
	})
}

// startOTPChallenge สร้างใบยืนยันใหม่ + ส่งรหัสทางอีเมล แล้วเขียน response ให้ client เอง
// ถูกเรียกจาก Login แทนการออก JWT เมื่อผู้ใช้ล็อกอินจากเครื่องที่ระบบยังไม่รู้จัก
//
// ใบเก่าที่ยังไม่ถูกใช้ของ user คนเดิมจะถูกทำให้หมดอายุทันที — ล็อกอินใหม่ = รหัสเก่าใช้ไม่ได้แล้ว
// (หลักการเดียวกับ generateResetToken ที่ลบลิงก์รีเซ็ตใบเก่าทิ้งก่อนออกใบใหม่)
func (h *AuthController) startOTPChallenge(c *gin.Context, db *gorm.DB, user *entity.User, remember bool) {
	// กวาดใบเก่าที่หมดประโยชน์แล้วทิ้งไปพร้อมกัน — พังก็แค่ log ไม่ต้องล้ม login
	if err := db.Where("created_at < ?", time.Now().Add(-otpChallengeRetention)).
		Delete(&entity.OTPChallenge{}).Error; err != nil {
		log.Printf("login: กวาด otp_challenges เก่าไม่สำเร็จ: %v", err)
	}

	var recent int64
	if err := db.Model(&entity.OTPChallenge{}).
		Where("user_id = ? AND created_at > ?", user.ID, time.Now().Add(-otpChallengeWindow)).
		Count(&recent).Error; err != nil {
		log.Printf("login: นับ otp_challenges ของ user %d ไม่สำเร็จ: %v", user.ID, err)
	} else if recent >= otpMaxChallengesPerWindow {
		utils.Error(c, http.StatusTooManyRequests, "OTP_RATE_LIMITED",
			"ขอรหัสยืนยันถี่เกินไป กรุณารอสักครู่แล้วลองใหม่อีกครั้ง")
		return
	}

	// ใบเก่าที่ยังมีชีวิตอยู่ให้ตายทันที ไม่งั้นรหัสจากอีเมลหลายฉบับจะใช้ได้พร้อมกัน
	if err := db.Model(&entity.OTPChallenge{}).
		Where("user_id = ? AND consumed_at IS NULL AND expires_at > ?", user.ID, time.Now()).
		Update("expires_at", time.Now()).Error; err != nil {
		log.Printf("login: ปิดใบ OTP เก่าของ user %d ไม่สำเร็จ: %v", user.ID, err)
	}

	code, codeHash, err := newOTPCode()
	if err != nil {
		log.Printf("login: สร้างรหัส OTP ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เริ่มการยืนยันตัวตนไม่สำเร็จ")
		return
	}
	plainToken, err := randomToken()
	if err != nil {
		log.Printf("login: สร้าง challenge token ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เริ่มการยืนยันตัวตนไม่สำเร็จ")
		return
	}

	now := time.Now()
	expiresAt := now.Add(otpTTL)
	challenge := entity.OTPChallenge{
		UserID:     user.ID,
		TokenHash:  hashToken(plainToken),
		CodeHash:   codeHash,
		Remember:   remember,
		ExpiresAt:  expiresAt,
		LastSentAt: now,
	}
	if err := db.Create(&challenge).Error; err != nil {
		log.Printf("login: สร้าง otp challenge ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เริ่มการยืนยันตัวตนไม่สำเร็จ")
		return
	}

	// ต่างจาก /forgot-password ที่ต้องปิดบังผลการส่ง (กันเดาว่ามีบัญชีนี้ไหม) — ตรงนี้ผู้ใช้กรอก
	// รหัสผ่านถูกมาแล้ว ไม่มีอะไรให้ปิดบัง บอกตรงๆ ดีกว่าปล่อยให้นั่งรอรหัสที่ไม่มีวันมา
	if err := h.mailer.SendOTPEmail(
		c.Request.Context(), user.Gmail, user.RealName, code, int(otpTTL.Minutes()),
	); err != nil {
		log.Printf("login: ส่งอีเมล OTP ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		// ลบใบที่เพิ่งสร้างทิ้ง เพราะไม่มีรหัสออกไปถึงมือใคร ถ้าปล่อยไว้จะไปกินโควตา
		// otpMaxChallengesPerWindow จนผู้ใช้โดนล็อก 15 นาทีเพราะ SMTP ล่ม ไม่ใช่เพราะตัวเอง
		if delErr := db.Delete(&challenge).Error; delErr != nil {
			log.Printf("login: ลบ challenge %d ที่ส่งเมลไม่ออกไม่สำเร็จ: %v", challenge.ID, delErr)
		}
		utils.Error(c, http.StatusBadGateway, "MAIL_FAILED",
			"ส่งอีเมลรหัสยืนยันไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{
		"otp_required":       true,
		"challenge_token":    plainToken,
		"gmail_masked":       maskEmail(user.Gmail),
		"expires_in_seconds": int(time.Until(expiresAt).Seconds()),
	})
}

// deviceTrusted บอกว่า device token ที่ client แนบมาเป็นเครื่องที่เคยผ่าน OTP ของ user คนนี้ไหม
// ถ้าใช่จะอัปเดต last_used_at ให้ด้วย แล้วคืน true ให้ Login ข้ามขั้น OTP
//
// เงื่อนไขต้องมี user_id เสมอ ไม่ใช่ดูแค่ว่า token มีอยู่จริง — ไม่งั้นใบผ่านของคนหนึ่ง
// ถูกเอาไปข้าม OTP ของอีกคนได้ ขอแค่รู้รหัสผ่านของเขา
func deviceTrusted(db *gorm.DB, userID int, plainToken string) bool {
	if plainToken == "" {
		return false
	}
	var device entity.TrustedDevice
	err := db.Where("user_id = ? AND token_hash = ? AND expires_at > ?",
		userID, hashToken(plainToken), time.Now()).First(&device).Error
	if err != nil {
		return false
	}
	if err := db.Model(&device).Update("last_used_at", time.Now()).Error; err != nil {
		log.Printf("login: อัปเดต last_used_at ของ trusted device %d ไม่สำเร็จ: %v", device.ID, err)
	}
	return true
}

// issueTrustedDevice ออกใบผ่านใบใหม่ให้เครื่องที่เพิ่งกรอก OTP ถูก — คืน token ตัวจริงให้ client เก็บ
// กวาดใบที่หมดอายุของ user คนนี้ทิ้งไปด้วย (เหตุผลเดียวกับที่กวาด otp_challenges ใน startOTPChallenge)
func issueTrustedDevice(db *gorm.DB, userID int) (string, error) {
	if err := db.Where("user_id = ? AND expires_at < ?", userID, time.Now()).
		Delete(&entity.TrustedDevice{}).Error; err != nil {
		log.Printf("verify-otp: กวาด trusted device หมดอายุของ user %d ไม่สำเร็จ: %v", userID, err)
	}

	plainToken, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	device := entity.TrustedDevice{
		UserID:     userID,
		TokenHash:  hashToken(plainToken),
		ExpiresAt:  now.Add(trustedDeviceTTL),
		LastUsedAt: now,
	}
	if err := db.Create(&device).Error; err != nil {
		return "", err
	}
	return plainToken, nil
}

// findLiveChallenge หาใบยืนยันที่ยัง "ใช้ได้จริง" จาก challenge token ที่ client ส่งมา
// ใช้ได้จริง = ยังไม่ถูกใช้ + ยังไม่หมดอายุ + ยังกรอกผิดไม่ครบเพดาน
// ไม่แยกสาเหตุที่ไม่ผ่านออกจากกัน (เหมือน consumeResetToken) — ผู้เรียกตอบข้อความเดียวเสมอ
func findLiveChallenge(db *gorm.DB, plainToken string) (*entity.OTPChallenge, error) {
	var challenge entity.OTPChallenge
	err := db.Where("token_hash = ? AND consumed_at IS NULL AND expires_at > ? AND attempts < ?",
		hashToken(plainToken), time.Now(), otpMaxAttempts).First(&challenge).Error
	if err != nil {
		return nil, err
	}
	return &challenge, nil
}

// findResendableChallenge หาใบที่ยัง "ขอรหัสใหม่ได้"
// ต่างจาก findLiveChallenge ตรงที่ยอมให้รหัสหมดอายุไปแล้ว (นั่นคือเหตุผลที่ผู้ใช้กดขอใหม่)
// แต่ใบต้องยังไม่ถูกใช้ ยังกรอกผิดไม่ครบเพดาน และยังไม่เกิน otpChallengeMaxLifetime
func findResendableChallenge(db *gorm.DB, plainToken string) (*entity.OTPChallenge, error) {
	var challenge entity.OTPChallenge
	err := db.Where("token_hash = ? AND consumed_at IS NULL AND attempts < ? AND created_at > ?",
		hashToken(plainToken), otpMaxAttempts, time.Now().Add(-otpChallengeMaxLifetime)).
		First(&challenge).Error
	if err != nil {
		return nil, err
	}
	return &challenge, nil
}

// newOTPCode สุ่มรหัส 6 หลัก คืนทั้งตัวจริง (ส่งอีเมล) และ bcrypt hash (เก็บลง DB)
//
// crypto/rand ไม่ใช่ math/rand — รหัสที่เดาลำดับได้เท่ากับไม่มีรหัส
// เติมศูนย์หน้าให้ครบ 6 หลักเสมอ ("007431" ไม่ใช่ "7431") ไม่งั้นช่องกรอกที่บังคับ 6 ตัวใช้ไม่ได้
func newOTPCode() (code, hash string, err error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", "", err
	}
	code = fmt.Sprintf("%06d", n.Int64())

	h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return code, string(h), nil
}

// randomToken สุ่ม token 32 ไบต์เป็น hex — ใช้ทั้ง challenge token และ device token
func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// maskEmail ปิดบังอีเมลให้เหลือแค่พอให้เจ้าตัวรู้ว่าไปดูกล่องไหน
// เช่น b6618452@g.sut.ac.th → b6****@g.sut.ac.th
//
// จำนวนดอกจันคงที่ ไม่ผูกกับความยาวจริง ไม่งั้นเท่ากับบอกความยาวอีเมลให้คนที่ขโมยรหัสผ่านมา
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "****"
	}
	local, domain := email[:at], email[at:]
	keep := 2
	if len(local) < keep {
		keep = len(local)
	}
	return local[:keep] + "****" + domain
}
