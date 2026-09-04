package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"backend/internal/config"
	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/mailer"
	"backend/internal/services"
	"backend/internal/utils"
)

// AuthController ดูแล register/login/me + ยืนยันอีเมล + รีเซ็ตรหัสผ่าน
type AuthController struct {
	db     *gorm.DB
	cfg    *config.Config
	mailer *mailer.Mailer
}

// NewAuthController ประกอบ controller พร้อม dependency — ถูกเรียกจาก router.Setup
// ผูก MailJournal ให้ทุกฉบับที่ส่งถูกบันทึกลง email_deliveries โดย handler ไม่ต้องรู้เรื่อง
func NewAuthController(db *gorm.DB, cfg *config.Config) *AuthController {
	return &AuthController{
		db:     db,
		cfg:    cfg,
		mailer: mailer.New(mailerConfigFrom(cfg), services.NewMailJournal(db)),
	}
}

// mailerConfigFrom แปลง config ของแอปเป็น config ของ mailer — จุดเดียวที่ทำ mapping นี้
// ให้ตัวที่ส่งจริงกับตัวที่ CheckMailer ทดสอบตอน start เป็นค่าชุดเดียวกันแน่นอน
func mailerConfigFrom(cfg *config.Config) mailer.Config {
	return mailer.Config{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		FromName: cfg.MailFromName,
		ReplyTo:  cfg.MailReplyTo,
	}
}

// CheckMailer ลองต่อ SMTP + ล็อกอินหนึ่งรอบตอน server start (ไม่ส่งเมลถึงใคร)
// คืน nil ถ้ายังไม่ได้ตั้งค่า SMTP ด้วย — ที่นี่สนใจแค่ "ตั้งค่าไว้แล้วแต่ใช้ไม่ได้จริง"
func CheckMailer(ctx context.Context, cfg *config.Config) error {
	m := mailer.New(mailerConfigFrom(cfg), nil)
	if !m.Configured() {
		return nil
	}
	return m.VerifyConnection(ctx)
}

// Register สมัครผู้ใช้ใหม่ — ผ่าน 4 ด่าน: มีใน eligible_students → เป็น CPE → สถานภาพยังเป็น นศ.
// → student_id/gmail ยังไม่ถูกใช้ แล้วจึงสร้าง user (gmail_verified_at = NULL) + ส่งลิงก์ยืนยัน
//
// ยังไม่ออก JWT ที่นี่ — JWT ออกตอนกดลิงก์ (ดู VerifyEmail)
//
// INSERT users + INSERT email_verifications + ส่งอีเมล อยู่ transaction เดียวกันโดยตั้งใจ:
// ส่งไม่ออก = rollback หมด "สมัครไม่สำเร็จ" จึงแปลว่าไม่มีอะไรถูกบันทึกจริงๆ ไม่มีบัญชีค้าง
// (เดิมสร้าง user เสร็จตอบ 201 แล้วค่อยขอ OTP พอขั้นนั้นพังจึงเหลือบัญชีที่สมัครซ้ำก็ไม่ได้ ล็อกอินก็ไม่ได้)
func (h *AuthController) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	req.Gmail = normalizeGmail(req.Gmail)

	db := h.db.WithContext(c.Request.Context())

	// ด่านที่ 1: ต้องเป็น student_id ที่มีอยู่ในฐานข้อมูลจริง (ทุกสาขา)
	var eligible entity.EligibleStudent
	if err := db.Where("student_id = ?", req.StudentID).First(&eligible).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Error(c, http.StatusForbidden, "STUDENT_NOT_FOUND",
				"ไม่พบรหัสนักศึกษานี้ในฐานข้อมูล")
			return
		}
		log.Printf("register: query eligible error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}

	// ด่านที่ 2: สมัครได้เฉพาะสาขา CPE เท่านั้น
	if eligible.Major != entity.MajorCPE {
		utils.Error(c, http.StatusForbidden, "NOT_CPE",
			"ระบบนี้เปิดให้เฉพาะนักศึกษาสาขาวิศวกรรมคอมพิวเตอร์ (CPE) เท่านั้น")
		return
	}

	// ด่านที่ 3: สถานภาพต้องยังเป็นนักศึกษาอยู่ (ไม่ใช่จบ/ลาพัก/พ้นสภาพ)
	if !entity.ActiveEnrollmentStatuses[eligible.EnrollmentStatus] {
		utils.Error(c, http.StatusForbidden, "NOT_ACTIVE_STUDENT",
			"สถานภาพนักศึกษาของรหัสนี้ไม่สามารถสมัครใช้งานได้")
		return
	}

	// เช็คก่อนแตะ DB: ส่งอีเมลไม่ได้ = สมัครไม่ได้ ไม่ต้องไปสร้างแล้ว rollback ทีหลัง
	if !h.mailer.Configured() {
		log.Println("register: ปฏิเสธการสมัครเพราะยังไม่ได้ตั้งค่า SMTP — ส่งลิงก์ยืนยันไม่ได้")
		utils.Error(c, http.StatusServiceUnavailable, "MAIL_UNAVAILABLE",
			"ระบบส่งอีเมลยังไม่พร้อมใช้งาน กรุณาลองใหม่ภายหลังหรือติดต่อผู้ดูแลระบบ")
		return
	}

	// ด่านที่ 4: เช็คซ้ำเองแทนการรอ unique constraint เพราะ error ของ Postgres
	// บอกไม่ได้ว่าชนที่ student_id หรือ gmail ซึ่งเป็นคนละคำแนะนำกันสำหรับผู้ใช้
	if handled := h.rejectDuplicateRegistration(c, db, req); handled {
		return
	}

	var userRole entity.Role
	if err := db.Where("name = ?", entity.RoleUser).First(&userRole).Error; err != nil {
		log.Printf("register: role '%s' หายไปจาก DB (ลืมรัน seed?): %v", entity.RoleUser, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ระบบยังตั้งค่าไม่ครบ")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "hash ไม่สำเร็จ")
		return
	}

	// ปีที่เข้าศึกษาไม่ใช่ค่า critical (ผ่านด่าน 1 มาแล้วแปลว่ารูปแบบรหัสน่าจะถูก) พังก็แค่ log ไม่ block
	entryYear, err := entity.EntryYearFromStudentID(req.StudentID)
	if err != nil {
		log.Printf("register: แกะปีที่เข้าศึกษาจาก student_id %q ไม่สำเร็จ: %v", req.StudentID, err)
	}

	user := entity.User{
		StudentID: req.StudentID,
		RoleID:    userRole.ID,
		RealName:  req.RealName,
		NickName:  req.NickName,
		Gmail:     req.Gmail,
		EntryYear: entryYear,
		Password:  string(hash),
		// GmailVerifiedAt เว้นเป็น NULL — ล็อกอินไม่ได้จนกว่าจะกดลิงก์ในอีเมล
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return h.sendVerificationLink(c.Request.Context(), tx, &user)
	})
	if err != nil {
		log.Printf("register: สมัคร student_id=%s ไม่สำเร็จ (ยกเลิกทั้งรายการแล้ว): %v", req.StudentID, err)
		if errors.Is(err, errVerificationMailFailed) {
			utils.Error(c, http.StatusBadGateway, "MAIL_FAILED",
				"ส่งอีเมลยืนยันไม่สำเร็จ จึงยังไม่ได้สร้างบัญชีให้ กรุณาตรวจสอบอีเมลแล้วลองใหม่อีกครั้ง")
			return
		}
		utils.Error(c, http.StatusConflict, "REGISTER_FAILED", "สมัครสมาชิกไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
		return
	}

	utils.OK(c, http.StatusCreated, pendingVerificationPayload(user.Gmail, verificationSentMsg))
}

// rejectDuplicateRegistration จัดการกรณี student_id หรือ gmail ถูกใช้ไปแล้ว
// คืน true เมื่อเขียน response ไปแล้ว (ผู้เรียกต้องหยุด) / false เมื่อสมัครต่อได้
//
// เคสที่ต้องแยกคือ "สมัครซ้ำด้วยข้อมูลชุดเดิมที่ยังไม่ยืนยัน" ซึ่งเกิดบ่อย (เมลตกสแปมแล้วกดสมัครใหม่)
// ตอบว่ารหัสซ้ำเท่ากับพาเข้าทางตัน จึงส่งลิงก์ใบใหม่ให้แทน
//
// ต้องเป็น "อีเมลเดิม" ด้วย ไม่ใช่แค่ student_id เดิม เพราะรหัส นศ. คนอื่นเดาได้ไม่ยาก — ถ้ายอมให้
// สมัครทับด้วยอีเมลอื่น จะแย่งบัญชีที่เจ้าตัวสมัครค้างไว้ไปผูกกับอีเมลตัวเองได้ก่อนเจ้าตัวกดลิงก์
func (h *AuthController) rejectDuplicateRegistration(c *gin.Context, db *gorm.DB, req dto.RegisterRequest) bool {
	var existing entity.User
	err := db.Where("student_id = ? OR lower(gmail) = ?", req.StudentID, req.Gmail).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		log.Printf("register: เช็คบัญชีซ้ำไม่สำเร็จ: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return true
	}

	sameStudent := existing.StudentID == req.StudentID
	sameGmail := strings.EqualFold(existing.Gmail, req.Gmail)

	if sameStudent && sameGmail && !existing.GmailVerified() {
		if err := h.sendVerificationLink(c.Request.Context(), db, &existing); err != nil {
			if errors.Is(err, errVerificationResendTooSoon) {
				utils.Error(c, http.StatusTooManyRequests, "RESEND_TOO_SOON",
					"เพิ่งส่งลิงก์ยืนยันไปเมื่อสักครู่ กรุณาตรวจกล่องอีเมล (รวมทั้งเมลขยะ) ก่อนลองใหม่")
				return true
			}
			log.Printf("register: ส่งลิงก์ยืนยันใบใหม่ให้ user %d ไม่สำเร็จ: %v", existing.ID, err)
			utils.Error(c, http.StatusBadGateway, "MAIL_FAILED",
				"ส่งอีเมลยืนยันไม่สำเร็จ กรุณาลองใหม่อีกครั้ง")
			return true
		}
		utils.OK(c, http.StatusOK, pendingVerificationPayload(existing.Gmail, verificationResentMsg))
		return true
	}

	if sameStudent {
		utils.Error(c, http.StatusConflict, "REGISTER_FAILED", "รหัสนักศึกษานี้สมัครไปแล้ว")
	} else {
		utils.Error(c, http.StatusConflict, "GMAIL_TAKEN", "อีเมลนี้ถูกใช้สมัครไปแล้ว")
	}
	return true
}

// ข้อความตอบกลับของ Register สองทางที่สำเร็จ — ทางหลังต้องบอกเรื่องรหัสผ่านเพราะการสมัครซ้ำ
// ไม่ได้ (และห้าม) เปลี่ยนรหัสผ่านของบัญชีเดิม ดูเหตุผลที่ rejectDuplicateRegistration
const (
	verificationSentMsg = "เราส่งลิงก์ยืนยันไปที่อีเมลของคุณแล้ว กดลิงก์ในอีเมลเพื่อเปิดใช้งานบัญชี " +
		"ถ้าไม่พบในกล่องขาเข้า กรุณาตรวจในเมลขยะ (Spam) ด้วย"

	verificationResentMsg = "บัญชีนี้เคยสมัครไว้แล้วแต่ยังไม่ได้ยืนยัน เราจึงส่งลิงก์ยืนยันใบใหม่ไปให้ " +
		"กดลิงก์ในอีเมลเพื่อเปิดใช้งานบัญชี (รหัสผ่านที่ใช้คือรหัสที่ตั้งไว้ตอนสมัครครั้งแรก) " +
		"ถ้าไม่พบในกล่องขาเข้า กรุณาตรวจในเมลขยะ (Spam) ด้วย"
)

// pendingVerificationPayload = รูปร่าง response ของทุกทางที่จบด้วย "ส่งลิงก์ยืนยันไปแล้ว"
// ข้อความต่างกันได้ แต่รูปร่างต้องเหมือนกัน หน้าเว็บจะได้ไม่ต้องแยกสองทาง
func pendingVerificationPayload(gmail, message string) gin.H {
	return gin.H{"gmail": gmail, "message": message}
}

// Login ตรวจรหัสผ่านแล้วออก JWT — บัญชีที่ยังไม่ยืนยันอีเมลได้ 403 EMAIL_NOT_VERIFIED
// (หน้าเว็บเอา code นั้นไปโชว์ช่องขอลิงก์ใหม่)
//
// "หา user ไม่เจอ" กับ "รหัสผิด" ตอบเหมือนกัน กันเดาว่ามี student_id นี้ไหม ส่วน "ยังไม่ยืนยัน"
// ตอบต่างโดยตั้งใจ — มาถึงตรงนี้ได้ต้องกรอกรหัสถูกแล้ว ไม่มีอะไรให้ปิดบัง และถ้ารวมกับ "รหัสผิด"
// ผู้ใช้จะไปนั่งรีเซ็ตรหัสผ่านทั้งที่รหัสถูกอยู่แล้ว
//
// ด่าน OTP + trusted device ถูกถอดออกทั้งชุด: ย้ายการยืนยันตัวตนไปไว้ที่ "ตอนสมัคร" ครั้งเดียว
// ซึ่งคือสิ่งที่ระบบนี้ต้องการจริง (ยืนยันว่าอีเมลเป็นของ นศ. คนนั้น) ไม่ใช่ 2FA เต็มรูปแบบ
func (h *AuthController) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	var user entity.User
	err := db.Where("student_id = ?", req.StudentID).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("login query error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		utils.Error(c, http.StatusUnauthorized, "LOGIN_FAILED", "student_id หรือ password ไม่ถูกต้อง")
		return
	}

	if !user.GmailVerified() {
		utils.Error(c, http.StatusForbidden, "EMAIL_NOT_VERIFIED",
			"บัญชีนี้ยังไม่ได้ยืนยันอีเมล กรุณากดลิงก์ยืนยันในอีเมลที่เราส่งไปให้ หรือกดขอลิงก์ใหม่ด้านล่าง")
		return
	}

	var role entity.Role
	if err := db.First(&role, user.RoleID).Error; err != nil {
		log.Printf("login: หา role ของ user %d ไม่เจอ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}

	payload, err := h.buildSession(&user, role.Name, req.Remember)
	if err != nil {
		log.Printf("login: สร้าง token ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง token ไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, payload)
}

// buildSession ประกอบ response ของการล็อกอินที่ผ่านครบทุกด่าน: JWT + ข้อมูล user
// แยกเป็นเมธอดเพราะรูปร่างนี้คือสัญญาที่หน้าเว็บยึดไว้ ต้องมีที่เดียวให้แก้เวลาเพิ่ม field
func (h *AuthController) buildSession(user *entity.User, roleName string, remember bool) (gin.H, error) {
	ttl := time.Duration(h.cfg.JWTTTLHours) * time.Hour
	if remember {
		ttl = time.Duration(h.cfg.JWTRememberTTLDays) * 24 * time.Hour
	}
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": roleName,
		"exp":  time.Now().Add(ttl).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	yearLevel, err := entity.YearLevel(user.StudentID, time.Now())
	if err != nil {
		log.Printf("login: คำนวณชั้นปีของ student_id %q ไม่สำเร็จ: %v", user.StudentID, err)
	}

	return gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"student_id":   user.StudentID,
			"real_name":    user.RealName,
			"nick_name":    user.NickName,
			"gmail":        user.Gmail,
			"year_level":   yearLevel,
			"role":         roleName,
			"namespace_id": user.NamespaceID,
		},
	}, nil
}

// Me คืนข้อมูลของผู้ใช้ที่ล็อกอินอยู่ — frontend เอา namespace_id ไปตัดสินใจว่าจะพาไปหน้า
// "สร้าง space" หรือหน้า dashboard
func (h *AuthController) Me(c *gin.Context) {
	var user entity.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, c.GetInt("userID")).Error; err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบผู้ใช้")
		return
	}

	yearLevel, err := entity.YearLevel(user.StudentID, time.Now())
	if err != nil {
		log.Printf("me: คำนวณชั้นปีของ student_id %q ไม่สำเร็จ: %v", user.StudentID, err)
	}

	utils.OK(c, http.StatusOK, gin.H{
		"id":           user.ID,
		"student_id":   user.StudentID,
		"real_name":    user.RealName,
		"nick_name":    user.NickName,
		"gmail":        user.Gmail,
		"year_level":   yearLevel,
		"role":         c.GetString("role"),
		"namespace_id": user.NamespaceID,
	})
}

// ตอบกลับ /forgot-password เสมอไม่ว่าจะมีอีเมลนี้ในระบบหรือไม่ กันการเดาว่ามีบัญชีอยู่ไหม
// หลักการเดียวกับ Login ที่ตอบ error เดียวสำหรับ "ไม่พบ user" กับ "รหัสผิด"
const genericForgotMsg = "ถ้ามีบัญชีที่ใช้อีเมลนี้ เราได้ส่งลิงก์รีเซ็ตรหัสผ่านไปให้แล้ว กรุณาตรวจสอบกล่องอีเมล"

// ForgotPassword สร้าง token แล้วส่งลิงก์รีเซ็ตไปทางอีเมล (มี rate limit ต่อ IP ที่ router)
// ตอบ genericForgotMsg 200 เสมอ แม้หา user ไม่เจอหรือส่งเมลพัง — ไม่รั่วว่ามีบัญชีนี้ไหม
func (h *AuthController) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	req.Gmail = normalizeGmail(req.Gmail)
	db := h.db.WithContext(c.Request.Context())

	var user entity.User
	err := db.Where("lower(gmail) = ?", req.Gmail).First(&user).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("forgot-password: query user error: %v", err)
		}
		utils.OK(c, http.StatusOK, gin.H{"message": genericForgotMsg})
		return
	}

	plainToken, err := h.generateResetToken(db, user.ID)
	if err != nil {
		log.Printf("forgot-password: generate token ให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.OK(c, http.StatusOK, gin.H{"message": genericForgotMsg})
		return
	}

	resetLink := strings.TrimRight(h.cfg.FrontendOrigin, "/") + "/reset-password?token=" + plainToken
	if err := h.mailer.SendPasswordResetEmail(
		c.Request.Context(), user.ID, user.Gmail, user.RealName, resetLink, h.cfg.ResetTokenTTLMinutes,
	); err != nil {
		log.Printf("forgot-password: ส่งอีเมลให้ user %d ไม่สำเร็จ: %v", user.ID, err)
	}

	utils.OK(c, http.StatusOK, gin.H{"message": genericForgotMsg})
}

// ResetPassword ตั้งรหัสผ่านใหม่จาก token ในลิงก์อีเมล
// token ผิด/หมดอายุ/ถูกใช้แล้ว ตอบ INVALID_TOKEN เหมือนกันหมด ไม่แยกสาเหตุ
func (h *AuthController) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	user, err := h.consumeResetToken(db, req.Token)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_TOKEN",
			"ลิงก์รีเซ็ตรหัสผ่านไม่ถูกต้องหรือหมดอายุแล้ว")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "hash ไม่สำเร็จ")
		return
	}

	// ตั้งรหัสใหม่ + นับเป็นการยืนยันอีเมลไปในตัว เพราะการกดลิงก์รีเซ็ตได้พิสูจน์แล้วว่าเปิดกล่องนั้นได้
	// ไม่งั้นคนที่สมัครค้างไว้แล้วลืมรหัสผ่านจะเข้าทางตัน — COALESCE กันไม่ให้ทับเวลายืนยันเดิม
	if err := db.Model(&entity.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{
			"password":          string(hash),
			"gmail_verified_at": gorm.Expr("COALESCE(gmail_verified_at, ?)", dbNow()),
		}).Error; err != nil {
		log.Printf("reset-password: อัปเดตรหัสผ่านของ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ตั้งรหัสผ่านใหม่ไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{
		"message": "ตั้งรหัสผ่านใหม่เรียบร้อยแล้ว กรุณาเข้าสู่ระบบด้วยรหัสผ่านใหม่",
	})
}

// generateResetToken สุ่ม token ใหม่ เก็บแต่ hash ลง DB คืน plain token ไว้ใส่ในลิงก์อีเมล
// ลบ token เก่าที่ยังไม่ถูกใช้ทิ้งก่อน — ขอใหม่ = ลิงก์เก่าตายทันที
func (h *AuthController) generateResetToken(db *gorm.DB, userID int) (string, error) {
	if err := db.Where("user_id = ? AND used_at IS NULL", userID).
		Delete(&entity.PasswordResetToken{}).Error; err != nil {
		return "", err
	}

	plainToken, err := randomToken()
	if err != nil {
		return "", err
	}

	row := entity.PasswordResetToken{
		UserID:    userID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: dbNow().Add(time.Duration(h.cfg.ResetTokenTTLMinutes) * time.Minute),
	}
	if err := db.Create(&row).Error; err != nil {
		return "", err
	}
	return plainToken, nil
}

// consumeResetToken ตรวจ token แล้ว mark ว่าใช้แล้ว คืน user ที่ผูกกับ token นั้น
// error ถ้า token ไม่ตรง / หมดอายุ / ถูกใช้ไปแล้ว — ไม่แยกสาเหตุเพื่อไม่ให้เดาสถานะ token ได้
func (h *AuthController) consumeResetToken(db *gorm.DB, plainToken string) (*entity.User, error) {
	tokenHash := hashToken(plainToken)

	var prt entity.PasswordResetToken
	if err := db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, dbNow()).
		First(&prt).Error; err != nil {
		return nil, err
	}

	now := dbNow()
	if err := db.Model(&prt).Update("used_at", &now).Error; err != nil {
		return nil, err
	}

	var user entity.User
	if err := db.First(&user, prt.UserID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// hashToken คืน sha256 hex ของ token — ใช้ทั้งตอนสร้าง (เก็บลง DB) และตอนตรวจ
func hashToken(plainToken string) string {
	sum := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(sum[:])
}
