package controller

import (
	"crypto/rand"
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
	"backend/internal/utils"
)

// AuthController ดูแล register/login/me + รีเซ็ตรหัสผ่าน
// ถือ db (query user), cfg (JWT secret / อายุ token / origin), และ mailer (ส่งอีเมลรีเซ็ต)
type AuthController struct {
	db     *gorm.DB
	cfg    *config.Config
	mailer *mailer.Mailer
}

// NewAuthController ประกอบ controller พร้อม dependency — ถูกเรียกจาก router.Setup
// สร้าง mailer จาก config ในตัว (ไม่ต้อง thread ผ่าน main/router เพิ่ม)
func NewAuthController(db *gorm.DB, cfg *config.Config) *AuthController {
	return &AuthController{
		db:     db,
		cfg:    cfg,
		mailer: mailer.New(cfg.ResendAPIKey, cfg.MailFrom),
	}
}

// Register สมัครผู้ใช้ใหม่ — เปิดให้เฉพาะ นศ. สาขา CPE ที่ยังมีสถานภาพเป็นนักศึกษาอยู่เท่านั้น
//
// data flow:
//   - JSON body → bind เป็น RegisterRequest
//   - ด่าน 1: เช็คว่ามี student_id นี้อยู่ในตาราง eligible_students ไหม (คือฐานข้อมูล นศ. ที่รู้จัก
//     ไม่ใช่แค่ CPE — ทุกสาขา) → ไม่เจอ → 403 STUDENT_NOT_FOUND
//   - ด่าน 2: เจอแล้วเช็คต่อว่า major ของคนนั้นตรงกับ entity.MajorCPE ไหม → ไม่ตรง → 403 NOT_CPE
//   - ด่าน 3: เช็คว่า enrollment_status ยังอยู่ใน entity.ActiveEnrollmentStatuses ไหม (จบ/ลาพัก/พ้นสภาพ
//     สมัครไม่ได้) → ไม่ผ่าน → 403 NOT_ACTIVE_STUDENT
//   - ผ่านทั้ง 3 ด่านแล้วค่อยหา role "user" จากตาราง roles เพื่อเอา role_id
//   - แกะปีที่เข้าศึกษาจาก prefix ของ student_id (entity.EntryYearFromStudentID) เก็บไว้ที่ user.EntryYear
//   - hash รหัสผ่านด้วย bcrypt → INSERT users (namespace_id ยังเป็น NULL — ไปสร้าง space ทีหลัง)
//   - ตอบข้อมูล user กลับ (ไม่ส่ง password)
//
// ผู้ใช้ที่เพิ่งสมัครจะยังไม่มี namespace ต้องไปเรียก POST /api/namespaces เพื่อสร้าง space ของตัวเองก่อน
// ถึงจะ deploy service ได้
func (h *AuthController) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

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

	// ด่านที่ 2: เจอ student_id แล้ว แต่สมัครได้เฉพาะสาขา CPE เท่านั้น
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

	// ปีที่เข้าศึกษาแกะจาก prefix ของ student_id ครั้งเดียวตอนนี้ — ไม่ใช่ค่า critical (ผ่านด่านที่ 1
	// มาแล้วแปลว่ารูปแบบรหัสน่าจะถูก) เลย error ได้แค่เก็บ log ไว้ ไม่ block การสมัคร
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
	}
	if err := db.Create(&user).Error; err != nil {
		utils.Error(c, http.StatusConflict, "REGISTER_FAILED", "รหัสนักศึกษานี้สมัครไปแล้ว")
		return
	}

	utils.OK(c, http.StatusCreated, gin.H{
		"id":         user.ID,
		"student_id": user.StudentID,
		"real_name":  user.RealName,
		"nick_name":  user.NickName,
		"gmail":      user.Gmail,
		"major":      eligible.Major,
	})
}

// Login ตรวจรหัสผ่านแล้วออก JWT
//
// data flow:
//   - JSON body → หา user จาก student_id → เทียบ bcrypt
//   - อ่านชื่อ role จากตาราง roles (ผ่าน role_id) เพื่อใส่ลง claim "role"
//   - เซ็น JWT (sub=id, role=ชื่อ role, exp = JWTTTLHours ปกติ หรือ JWTRememberTTLDays ถ้าติ๊ก remember) → ตอบ token + ข้อมูล user
//
// error ของ "หา user ไม่เจอ" กับ "รหัสผิด" ตอบเหมือนกัน เพื่อไม่ให้เดาได้ว่ามี student_id นี้ในระบบหรือไม่
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

	var role entity.Role
	if err := db.First(&role, user.RoleID).Error; err != nil {
		log.Printf("login: หา role ของ user %d ไม่เจอ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}

	ttl := time.Duration(h.cfg.JWTTTLHours) * time.Hour
	if req.Remember {
		ttl = time.Duration(h.cfg.JWTRememberTTLDays) * 24 * time.Hour
	}
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": role.Name,
		"exp":  time.Now().Add(ttl).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง token ไม่สำเร็จ")
		return
	}

	yearLevel, err := entity.YearLevel(user.StudentID, time.Now())
	if err != nil {
		log.Printf("login: คำนวณชั้นปีของ student_id %q ไม่สำเร็จ: %v", user.StudentID, err)
	}

	utils.OK(c, http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"student_id":   user.StudentID,
			"real_name":    user.RealName,
			"nick_name":    user.NickName,
			"gmail":        user.Gmail,
			"year_level":   yearLevel,
			"role":         role.Name,
			"namespace_id": user.NamespaceID,
		},
	})
}

// Me คืนข้อมูลของผู้ใช้ที่ล็อกอินอยู่ (ให้ frontend รู้ว่ามี namespace แล้วหรือยัง)
// data flow: อ่าน userID ที่ middleware Auth ตั้งไว้ → SELECT users → ตอบข้อมูล + namespace_id
// frontend เอา namespace_id ไปตัดสินใจว่าจะพาไปหน้า "สร้าง space" หรือหน้า dashboard
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

// genericForgotMsg = ข้อความที่ตอบกลับ /forgot-password เสมอ ไม่ว่าจะมี email นี้ในระบบหรือไม่
// จงใจให้เหมือนกันทุกกรณีเพื่อกันการเดาว่ามีบัญชีนี้อยู่ไหม (user enumeration) —
// หลักการเดียวกับ Login ที่ตอบ error เดียวสำหรับ "ไม่พบ user" กับ "รหัสผิด"
const genericForgotMsg = "ถ้ามีบัญชีที่ใช้อีเมลนี้ เราได้ส่งลิงก์รีเซ็ตรหัสผ่านไปให้แล้ว กรุณาตรวจสอบกล่องอีเมล"

// ForgotPassword รับอีเมล → ถ้ามี user จริงก็สร้าง token แล้วส่งลิงก์รีเซ็ตไปทางอีเมล
//
// data flow:
//   - JSON body → bind ForgotPasswordRequest (ต้องเป็น email)
//   - หา user จาก gmail — ไม่ว่าจะเจอหรือไม่ ตอบ genericForgotMsg (200) เหมือนกันเป๊ะ กันเดาว่ามีบัญชีไหม
//   - ถ้าเจอ: generate token (ลบ token เก่าที่ยังไม่ใช้ทิ้งก่อน) → ประกอบลิงก์ FRONTEND_ORIGIN/reset-password?token=...
//     → ส่งอีเมลผ่าน mailer
//   - ถ้า mailer พัง (เช่นยังไม่ได้ตั้ง RESEND_API_KEY) แค่ log ไว้ ยังตอบ 200 generic ไม่รั่วให้ client รู้
//
// route นี้มี rate limit ต่อ IP (ดู router.Setup) กันสแปม/email-bombing
func (h *AuthController) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	db := h.db.WithContext(c.Request.Context())

	var user entity.User
	err := db.Where("gmail = ?", req.Gmail).First(&user).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("forgot-password: query user error: %v", err)
		}
		// ไม่เจอ user (หรือ query พัง) ก็ตอบ generic เหมือนกัน ไม่บอกให้ client รู้
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
		c.Request.Context(), user.Gmail, user.RealName, resetLink, h.cfg.ResetTokenTTLMinutes,
	); err != nil {
		log.Printf("forgot-password: ส่งอีเมลให้ user %d ไม่สำเร็จ: %v", user.ID, err)
		// ยังตอบ 200 generic ตามเดิม ไม่รั่วรายละเอียดผ่าน error response
	}

	utils.OK(c, http.StatusOK, gin.H{"message": genericForgotMsg})
}

// ResetPassword ตั้งรหัสผ่านใหม่จาก token ที่อยู่ในลิงก์อีเมล
//
// data flow:
//   - JSON body → bind ResetPasswordRequest (token + new_password min=8)
//   - consumeResetToken: hash token → หาแถวที่ยังไม่ถูกใช้ + ยังไม่หมดอายุ → mark used → คืน user
//     ไม่ผ่าน (ผิด/หมดอายุ/ใช้ไปแล้ว) → 400 INVALID_TOKEN (ข้อความเดียวไม่แยกสาเหตุ)
//   - bcrypt hash รหัสใหม่ (เหมือน Register) → UPDATE users.password
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

	if err := db.Model(&entity.User{}).Where("id = ?", user.ID).
		Update("password", string(hash)).Error; err != nil {
		log.Printf("reset-password: อัปเดตรหัสผ่านของ user %d ไม่สำเร็จ: %v", user.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ตั้งรหัสผ่านใหม่ไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{
		"message": "ตั้งรหัสผ่านใหม่เรียบร้อยแล้ว กรุณาเข้าสู่ระบบด้วยรหัสผ่านใหม่",
	})
}

// generateResetToken สุ่ม token ใหม่ให้ user แล้วเก็บแต่ hash ลง DB — คืน plain token ไว้ใส่ในลิงก์อีเมล
//
// ลบ token เก่าที่ยังไม่ถูกใช้ของ user นี้ทิ้งก่อน กันมีลิงก์ค้างหลายใบใช้ได้พร้อมกัน (ขอใหม่ = ลิงก์เก่าตายทันที)
func (h *AuthController) generateResetToken(db *gorm.DB, userID int) (string, error) {
	if err := db.Where("user_id = ? AND used_at IS NULL", userID).
		Delete(&entity.PasswordResetToken{}).Error; err != nil {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	plainToken := hex.EncodeToString(raw)
	tokenHash := hashToken(plainToken)

	ttl := time.Duration(h.cfg.ResetTokenTTLMinutes) * time.Minute
	row := entity.PasswordResetToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := db.Create(&row).Error; err != nil {
		return "", err
	}
	return plainToken, nil
}

// consumeResetToken ตรวจ token ที่รับมา (plain) แล้ว mark ว่าใช้แล้ว — คืน user ที่ผูกกับ token นั้น
// error ถ้า: token ไม่ตรง / หมดอายุ / ถูกใช้ไปแล้ว — ไม่แยกสาเหตุเพื่อไม่ให้เดาสถานะ token ได้
func (h *AuthController) consumeResetToken(db *gorm.DB, plainToken string) (*entity.User, error) {
	tokenHash := hashToken(plainToken)

	var prt entity.PasswordResetToken
	if err := db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).
		First(&prt).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	if err := db.Model(&prt).Update("used_at", &now).Error; err != nil {
		return nil, err
	}

	var user entity.User
	if err := db.First(&user, prt.UserID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// hashToken คืน sha256 hex ของ token — ใช้ทั้งตอนสร้าง (เก็บลง DB) และตอนตรวจ (เทียบกับที่เก็บไว้)
func hashToken(plainToken string) string {
	sum := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(sum[:])
}
