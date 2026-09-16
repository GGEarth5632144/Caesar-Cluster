package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/extrame/xls"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/config"
	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/mailer"
	"backend/internal/services"
	"backend/internal/utils"
)

// errRequestNotPending = internal sentinel ใช้เทียบใน Approve เมื่อคำขอถูกจัดการไปแล้ว
var errRequestNotPending = errors.New("request ถูกดำเนินการไปแล้ว")

// AdminController รวม endpoint ฝั่ง admin ไว้ที่เดียว:
// import รายชื่อ นศ. ที่มีสิทธิ์, สร้าง choices (plans), ดูภาพรวม namespace, ปรับโควตาให้กลุ่ม
// ทุก route ที่ผูกกับ controller นี้ผ่าน middleware AdminOnly มาแล้ว
// svc ใช้เฉพาะตอน DeleteUser — ต้องถอน service ที่ user ทิ้งไว้ใน space ของคนอื่นออกจากคลัสเตอร์
// ก่อนที่ FK จะลบแถวมันหายไปเงียบๆ (ดูเหตุผลเต็มใน DeleteUser)
// mailer ใช้แจ้งสมาชิกตอนแอดมินลบ namespace (ดู DeleteNamespace)
type AdminController struct {
	db     *gorm.DB
	cfg    *config.Config
	ns     *services.NamespaceManager
	svc    *services.ServiceManager
	mailer *mailer.Mailer
}

// NewAdminController ประกอบ controller — ถูกเรียกจาก router.Setup
// mailer ประกอบแบบเดียวกับ NewAuthController ทุกฉบับจึงถูกบันทึกลง email_deliveries เหมือนกัน
func NewAdminController(db *gorm.DB, cfg *config.Config, ns *services.NamespaceManager, svc *services.ServiceManager) *AdminController {
	return &AdminController{
		db:     db,
		cfg:    cfg,
		ns:     ns,
		svc:    svc,
		mailer: mailer.New(mailerConfigFrom(cfg), services.NewMailJournal(db)),
	}
}

// ListEligibleStudents คืนรายชื่อ นศ. ที่มีสิทธิ์ทั้งหมด (ตาราง "match") ให้ admin ตรวจสอบ
// ว่า import เข้ามาแล้วใครเป็นยังไงบ้าง — เรียงตาม imported_at ล่าสุดก่อน (เห็นรายชื่อที่เพิ่ง
// import/อัปเดตล่าสุดอยู่บนสุด) ไม่มี pagination/filter ฝั่ง server เพราะจำนวนแถวเป็นระดับ นศ.
// ทั้งคณะ ไม่ใหญ่พอที่ต้องแบ่งหน้า ฝั่ง frontend กรอง/ค้นหาเอาเองพอ
func (h *AdminController) ListEligibleStudents(c *gin.Context) {
	var students []entity.EligibleStudent
	if err := h.db.WithContext(c.Request.Context()).
		Order("imported_at DESC").Find(&students).Error; err != nil {
		log.Printf("list eligible students error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงรายชื่อผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, students)
}

// AddEligibleStudent เพิ่ม/อัปเดตผู้มีสิทธิ์ทีละคนจากหน้า User Management พร้อมกำหนด role
//
// data flow: JSON body → bind SingleEligibleStudentRequest → ใน transaction เดียว:
// UPSERT eligible_students (real_name เขียนทับเฉพาะเมื่อกรอกมา) → ถ้ารหัสนี้สมัครเป็นผู้ใช้แล้ว
// UPDATE users.role_id ให้ตรงกับ role ที่เลือก → ตอบแถวล่าสุด + บอกว่าเปลี่ยน role ของบัญชีที่มีอยู่หรือไม่
//
// ต้องอัปเดตบัญชีด้วย ไม่งั้นการเลือก role ให้คนที่สมัครไปแล้วจะไม่มีผลอะไร (Register อ่าน role จาก
// ตารางนี้ครั้งเดียวตอนสมัคร) ส่วนคนที่ยังไม่สมัครจะได้ role นี้ตอนสมัคร
func (h *AdminController) AddEligibleStudent(c *gin.Context) {
	var req dto.SingleEligibleStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	studentID := strings.TrimSpace(req.StudentID)
	if studentID == "" {
		utils.Error(c, http.StatusBadRequest, "INVALID_STUDENT_ID", "กรุณากรอกรหัสประจำตัว")
		return
	}
	major, ok := eligibleMajor(c, req)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	role, ok := h.resolveEligibleRole(c, studentID, req.Role)
	if !ok {
		return
	}

	row := entity.EligibleStudent{
		StudentID:        studentID,
		RealName:         strings.TrimSpace(req.RealName),
		Major:            major,
		EnrollmentStatus: req.EnrollmentStatus,
		Role:             req.Role,
		ImportedAt:       time.Now(),
	}
	updateColumns := []string{"major", "enrollment_status", "role", "imported_at"}
	if row.RealName != "" {
		updateColumns = append(updateColumns, "real_name")
	}
	var userRoleUpdated bool
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "student_id"}},
			DoUpdates: clause.AssignmentColumns(updateColumns),
		}).Create(&row).Error; err != nil {
			return err
		}
		if req.Role == entity.RoleAdmin {
			if err := tx.Model(&entity.EligibleStudent{}).Where("student_id = ?", studentID).
				Update("enrollment_status", 0).Error; err != nil {
				return err
			}
		}
		var err error
		userRoleUpdated, err = syncUserRole(tx, studentID, role.ID)
		return err
	})
	if err != nil {
		log.Printf("add eligible student %q error: %v", studentID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เพิ่มผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	// อ่านกลับเพื่อให้ได้ชื่อเดิม/created_at จริงของแถวที่มีอยู่แล้ว — อ่านไม่ได้ก็ตอบค่าที่เพิ่งเขียนแทน
	saved := row
	if err := h.db.WithContext(ctx).Where("student_id = ?", studentID).First(&saved).Error; err != nil {
		log.Printf("add eligible student: re-read %q error: %v", studentID, err)
	}
	utils.OK(c, http.StatusCreated, gin.H{
		"student":           saved,
		"user_role_updated": userRoleUpdated,
	})
}

func (h *AdminController) UpdateEligibleStudent(c *gin.Context) {
	oldID := c.Param("studentId")
	var req dto.SingleEligibleStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	newID := strings.TrimSpace(req.StudentID)
	if newID == "" {
		utils.Error(c, http.StatusBadRequest, "INVALID_STUDENT_ID", "กรุณากรอกรหัสประจำตัว")
		return
	}
	major, ok := eligibleMajor(c, req)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	var current entity.EligibleStudent
	if err := h.db.WithContext(ctx).Where("student_id = ?", oldID).First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบรายชื่อนี้")
			return
		}
		log.Printf("update eligible student %q: load error: %v", oldID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "แก้ไขผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	role, ok := h.resolveEligibleRole(c, oldID, req.Role)
	if !ok {
		return
	}

	if newID != oldID {
		var taken int64
		if err := h.db.WithContext(ctx).Model(&entity.EligibleStudent{}).
			Where("LOWER(student_id) = LOWER(?) AND student_id <> ?", newID, oldID).
			Count(&taken).Error; err != nil {
			log.Printf("update eligible student %q: check duplicate error: %v", oldID, err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "แก้ไขผู้มีสิทธิ์ไม่สำเร็จ")
			return
		}
		if taken > 0 {
			utils.Error(c, http.StatusConflict, "STUDENT_ID_TAKEN", "รหัสประจำตัวนี้มีในรายชื่ออยู่แล้ว")
			return
		}
	}

	status := req.EnrollmentStatus
	if req.Role == entity.RoleAdmin {
		status = 0
	}

	var userUpdated bool
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if newID != oldID {
			moved := current
			moved.StudentID = newID
			if err := tx.Create(&moved).Error; err != nil {
				return err
			}
			res := tx.Model(&entity.User{}).Where("student_id = ?", oldID).Update("student_id", newID)
			if res.Error != nil {
				return res.Error
			}
			userUpdated = res.RowsAffected > 0
			if err := tx.Model(&entity.NamespaceInvite{}).Where("invited_student_id = ?", oldID).
				Update("invited_student_id", newID).Error; err != nil {
				return err
			}
			if err := tx.Where("student_id = ?", oldID).Delete(&entity.EligibleStudent{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&entity.EligibleStudent{}).Where("student_id = ?", newID).Updates(map[string]any{
			"real_name":         strings.TrimSpace(req.RealName),
			"major":             major,
			"enrollment_status": status,
			"role":              req.Role,
		}).Error; err != nil {
			return err
		}
		roleUpdated, err := syncUserRole(tx, newID, role.ID)
		userUpdated = userUpdated || roleUpdated
		return err
	})
	if err != nil {
		log.Printf("update eligible student %q error: %v", oldID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "แก้ไขผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	var saved entity.EligibleStudent
	if err := h.db.WithContext(ctx).Where("student_id = ?", newID).First(&saved).Error; err != nil {
		log.Printf("update eligible student: re-read %q error: %v", newID, err)
	}
	utils.OK(c, http.StatusOK, gin.H{
		"student":      saved,
		"user_updated": userUpdated,
	})
}

func (h *AdminController) DeleteEligibleStudent(c *gin.Context) {
	studentID := c.Param("studentId")
	ctx := c.Request.Context()

	if h.isSelf(c, studentID) {
		utils.Error(c, http.StatusBadRequest, "CANNOT_DELETE_SELF", "ไม่สามารถลบรายชื่อของตัวเองได้")
		return
	}

	var row entity.EligibleStudent
	if err := h.db.WithContext(ctx).Where("student_id = ?", studentID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบรายชื่อนี้")
			return
		}
		log.Printf("delete eligible student %q: load error: %v", studentID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	var user entity.User
	userDeleted := false
	err := h.db.WithContext(ctx).Where("student_id = ?", studentID).First(&user).Error
	switch {
	case err == nil:
		if msg, err := h.deleteUserAccount(ctx, user.ID); err != nil {
			log.Printf("delete eligible student %q: delete user %d: %v", studentID, user.ID, err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", msg)
			return
		}
		userDeleted = true
	case !errors.Is(err, gorm.ErrRecordNotFound):
		log.Printf("delete eligible student %q: load user error: %v", studentID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("invited_student_id = ?", studentID).Delete(&entity.NamespaceInvite{}).Error; err != nil {
			return err
		}
		return tx.Where("student_id = ?", studentID).Delete(&entity.EligibleStudent{}).Error
	})
	if err != nil {
		log.Printf("delete eligible student %q error: %v", studentID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบผู้มีสิทธิ์ไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"student_id": studentID, "user_deleted": userDeleted})
}

func (h *AdminController) resolveEligibleRole(c *gin.Context, studentID, roleName string) (entity.Role, bool) {
	var role entity.Role
	// กันแอดมินลดสิทธิ์ตัวเองจากฟอร์มนี้ — กดพลาดครั้งเดียวคือหลุดจากหน้าแอดมินทันทีและแก้คืนเองไม่ได้
	if roleName != entity.RoleAdmin && h.isSelf(c, studentID) {
		utils.Error(c, http.StatusBadRequest, "CANNOT_CHANGE_OWN_ROLE", "ไม่สามารถแก้ไข Role ของตัวเองได้")
		return role, false
	}
	if err := h.db.WithContext(c.Request.Context()).Where("name = ?", roleName).First(&role).Error; err != nil {
		log.Printf("eligible student: role '%s' หายไปจาก DB (ลืมรัน seed?): %v", roleName, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ระบบยังตั้งค่าไม่ครบ")
		return role, false
	}
	return role, true
}

func eligibleMajor(c *gin.Context, req dto.SingleEligibleStudentRequest) (string, bool) {
	if req.Role == entity.RoleAdmin {
		return "", true
	}
	major := strings.TrimSpace(req.Major)
	if utf8.RuneCountInString(major) < 2 {
		utils.Error(c, http.StatusBadRequest, "INVALID_MAJOR", "กรุณากรอกสาขาวิชาอย่างน้อย 2 ตัวอักษร")
		return "", false
	}
	return major, true
}

func (h *AdminController) isSelf(c *gin.Context, studentID string) bool {
	var me entity.User
	err := h.db.WithContext(c.Request.Context()).First(&me, c.GetInt("userID")).Error
	return err == nil && me.StudentID == studentID
}

func syncUserRole(tx *gorm.DB, studentID string, roleID int) (bool, error) {
	res := tx.Model(&entity.User{}).
		Where("student_id = ? AND role_id <> ?", studentID, roleID).
		Update("role_id", roleID)
	return res.RowsAffected > 0, res.Error
}

// AddEligibleStudents = ขั้น "confirm" ของการ import รายชื่อ นศ. ที่มีสิทธิ์สมัครใช้งาน
// (ตาราง "match" ใน ERD) — รับ list ที่ผ่านการ preview+validate มาแล้วจาก
// POST /api/admin/eligible-students/preview (ดู PreviewEligibleStudents ด้านล่าง)
//
// data flow: JSON body (array) → bind AddEligibleStudentRequest
// → UPSERT eligible_students (ซ้ำ student_id เดิม → อัปเดต major/real_name/enrollment_status/imported_at
// ให้ตรงไฟล์ล่าสุด แทนที่จะข้ามเฉยๆ) → ตอบจำนวนที่ insert/update จริง
//
// จงใจ "ไม่ลบ" แถวที่หายไปจากไฟล์ใหม่ — ลบไม่ได้เพราะ users.student_id มี FK อ้างมาที่ตารางนี้
// (นศ. จบ/พ้นสภาพ ให้ปรับ enrollment_status ในไฟล์ที่ import เข้ามาแทน ไม่ใช่ตัดชื่อออกจากไฟล์)
//
// นี่คือประตูเดียวที่ทำให้ใครสมัครได้ — ถ้า student_id ไม่อยู่ในตารางนี้ Register จะตอบ 403 เสมอ
func (h *AdminController) AddEligibleStudents(c *gin.Context) {
	var req dto.AddEligibleStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	now := time.Now()
	rows := make([]entity.EligibleStudent, 0, len(req.Students))
	for _, s := range req.Students {
		rows = append(rows, entity.EligibleStudent{
			StudentID:        s.StudentID,
			RealName:         s.RealName,
			Major:            s.Major,
			EnrollmentStatus: s.EnrollmentStatus,
			ImportedAt:       now,
		})
	}

	res := h.db.WithContext(c.Request.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "student_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"real_name", "major", "enrollment_status", "imported_at"}),
	}).Create(&rows)
	if res.Error != nil {
		log.Printf("add eligible students error: %v", res.Error)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เพิ่มรายชื่อไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusCreated, gin.H{
		"submitted": len(rows),
		"upserted":  res.RowsAffected,
	})
}

// eligibleExcelHeaders = ชื่อคอลัมน์ (แถวแรกของไฟล์) ที่ต้องมีในไฟล์ export จากทะเบียน
// ใช้ชื่อคอลัมน์หา index แทนการ hardcode ตัวอักษรคอลัมน์ (B, C, D, ...) กันพังถ้าทะเบียนสลับลำดับคอลัมน์
var eligibleExcelHeaders = struct {
	studentID string
	realName  string
	major     string
	status    string
}{
	studentID: "รหัสประจำตัว",
	realName:  "ชื่อ-สกุล",
	major:     "สาขาวิชา",
	status:    "สถานภาพ",
}

// ole2Signature = magic bytes ของไฟล์ OLE2/Compound File Binary — ทุกไฟล์ .xls แบบ binary จริง
// (Excel 97-2003 / BIFF8) ต้องขึ้นต้นด้วย byte ชุดนี้เสมอ ใช้แยกจากไฟล์ HTML ที่แค่ตั้งชื่อ .xls
var ole2Signature = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// readSpreadsheetRows เปิดไฟล์ที่ admin อัปโหลดแล้วอ่านออกมาเป็น [][]string (แถว x คอลัมน์) ของชีตแรก
//
// เลือก parser ตาม "เนื้อหาจริง" ของไฟล์ ไม่ใช่แค่นามสกุล เพราะระบบทะเบียนหลายเจ้า (โดยเฉพาะที่ทำจาก
// ASP.NET) "export เป็น Excel" ด้วยการ render ตาราง HTML ธรรมดาแล้วตั้งชื่อไฟล์ลงท้าย .xls ให้เฉยๆ —
// Windows/Excel เปิดได้ปกติและขึ้น type "Excel 97-2003 Worksheet" เหมือนไฟล์จริงทุกอย่าง แต่ไม่ใช่
// binary format (BIFF8) เลย ต้อง parse เป็น HTML table แทน ไม่งั้น excelize/extrame/xls จะอ่านไม่ออก
func readSpreadsheetRows(fileHeader *multipart.FileHeader) ([][]string, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("เปิดไฟล์ไม่สำเร็จ")
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
	case ".xlsx", ".xlsm":
		rows, xlsxErr := readXLSXRows(f)
		if xlsxErr == nil {
			return rows, nil
		}
		// เผื่อไฟล์ตั้งชื่อ .xlsx แต่จริงๆ เป็น HTML เหมือนกัน — ลองอ่านเป็น HTML table ก่อนยอมแพ้
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
			if htmlRows, htmlErr := readHTMLTableRows(f); htmlErr == nil {
				return htmlRows, nil
			}
		}
		return nil, xlsxErr
	case ".xls":
		head := make([]byte, len(ole2Signature))
		n, _ := io.ReadFull(f, head)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("อ่านไฟล์ไม่สำเร็จ")
		}
		if n == len(ole2Signature) && bytes.Equal(head, ole2Signature) {
			return readXLSRows(f)
		}
		return readHTMLTableRows(f)
	default:
		return nil, fmt.Errorf("รองรับเฉพาะไฟล์ .xlsx หรือ .xls เท่านั้น")
	}
}

// readXLSXRows อ่านไฟล์ .xlsx (OOXML) ด้วย excelize
func readXLSXRows(f io.Reader) ([][]string, error) {
	xl, err := excelize.OpenReader(f)
	if err != nil {
		return nil, fmt.Errorf("ไฟล์นี้ไม่ใช่ไฟล์ Excel (.xlsx) ที่อ่านได้")
	}
	defer xl.Close()

	sheets := xl.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("ไฟล์นี้ไม่มีชีตข้อมูล")
	}
	rows, err := xl.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("อ่านข้อมูลในไฟล์ไม่สำเร็จ")
	}
	return rows, nil
}

// readXLSRows อ่านไฟล์ .xls แบบเก่า (Excel 97-2003 / BIFF8) ด้วย github.com/extrame/xls
func readXLSRows(f io.ReadSeeker) ([][]string, error) {
	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("ไฟล์นี้ไม่ใช่ไฟล์ Excel (.xls) ที่อ่านได้")
	}
	if wb.NumSheets() == 0 {
		return nil, fmt.Errorf("ไฟล์นี้ไม่มีชีตข้อมูล")
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, fmt.Errorf("อ่านชีตแรกไม่สำเร็จ")
	}

	rows := make([][]string, 0, int(sheet.MaxRow)+1)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		cells, ok := readXLSRow(sheet, i)
		if !ok {
			// extrame/xls panic เวลาเจอแถวที่ไม่มี cell เลย (ว่างสนิท) — ถือว่าตารางข้อมูลจบตรงนี้
			// (ไฟล์จากทะเบียนมักมีแถวว่างคั่นก่อนถึงแถว legend ท้ายไฟล์พอดี ดู PreviewEligibleStudents)
			break
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

// readXLSRow อ่าน 1 แถวแบบกันพัง — extrame/xls (unmaintained) dereference nil ตอนแถวไม่มีข้อมูลเลย
func readXLSRow(sheet *xls.WorkSheet, i int) (cells []string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	row := sheet.Row(i)
	if row == nil {
		return nil, false
	}
	last := row.LastCol()
	cells = make([]string, 0, last+1)
	for c := 0; c <= last; c++ {
		cells = append(cells, row.Col(c))
	}
	return cells, true
}

// readHTMLTableRows อ่านไฟล์ "Excel" ที่จริงๆ เป็น HTML table (ดูคอมเมนต์ที่ readSpreadsheetRows)
// เช่นไฟล์ export ของระบบทะเบียนที่มี <meta http-equiv=Content-Type content="text/html; charset=windows-874">
// — charset.NewReader สแกนหา meta tag นี้แล้วแปลงเป็น UTF-8 ให้อัตโนมัติ (windows-874 คือ codepage
// ภาษาไทยที่ระบบเก่าๆ นิยมใช้) ก่อนส่งต่อให้ html.Parse
//
// เก็บ <tr> จากทั้งเอกสาร ไม่ยึดกับ <table> เดียว เพราะไฟล์จริงจากทะเบียนบางระบบ export ออกมาโดยห่อ
// แต่ละแถวด้วย <table> แยกกันคนละอัน (พบว่ามี <table> ~24 อันในไฟล์ 24 แถวข้อมูล) ไม่ใช่ตารางเดียว
// ที่มีหลาย <tr> ข้างใน — เดินทั้งเอกสารแล้วเก็บทุก <tr> ตามลำดับที่เจอ จะได้ผลถูกต้องทั้งสองแบบ
func readHTMLTableRows(f io.Reader) ([][]string, error) {
	utf8Reader, err := charset.NewReader(f, "text/html")
	if err != nil {
		return nil, fmt.Errorf("อ่านไฟล์ไม่สำเร็จ")
	}
	doc, err := html.Parse(utf8Reader)
	if err != nil {
		return nil, fmt.Errorf("ไฟล์นี้ไม่ใช่ไฟล์ Excel (.xlsx/.xls) หรือ HTML table ที่อ่านได้")
	}

	rows := htmlTableRows(doc)
	if len(rows) == 0 {
		return nil, fmt.Errorf("ไม่พบข้อมูลในตาราง")
	}
	return rows, nil
}

// htmlTableRows เก็บทุก <tr> ที่อยู่ใต้ root ที่ให้มา (ทั้งเอกสาร) เป็น [][]string — 1 แถวต่อ <tr>,
// 1 ช่องต่อ <td>/<th> — จงใจไม่จำกัดว่าต้องอยู่ใน <table> เดียวกัน (ดูเหตุผลที่ readHTMLTableRows)
// ใช้ htmlNodeText ดึงข้อความออกมาแบบ recursive เพราะ Excel มักห่อข้อความในแต่ละช่องด้วย <font>/<span> อีกที
func htmlTableRows(root *html.Node) [][]string {
	var rows [][]string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, strings.TrimSpace(htmlNodeText(c)))
				}
			}
			rows = append(rows, cells)
			return // แถวใน Excel HTML export ไม่ควรมี <tr> ซ้อนกันเองอยู่แล้ว ไม่ต้องลงไปลึกกว่านี้
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return rows
}

// htmlNodeText รวมข้อความของ text node ทั้งหมดใต้ n เข้าด้วยกัน (ไล่ลงไปทุกชั้นของ <font>/<span> ที่ห่ออยู่)
func htmlNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// PreviewEligibleStudents อ่านไฟล์ Excel ที่ admin อัปโหลด (export จากระบบทะเบียน) แล้ว parse+validate
// ให้ดูก่อนว่าจะเกิดอะไรขึ้นบ้าง โดยยังไม่เขียนอะไรลง DB
//
// data flow: multipart file (.xlsx หรือ .xls) → readSpreadsheetRows เลือก parser ตามนามสกุลไฟล์
// → อ่านแถวแรกเป็น header หา index ของแต่ละคอลัมน์
// → ไล่ทีละแถวจนกว่าจะเจอแถวที่รหัสประจำตัวว่าง (จุดที่ข้อมูลตารางจบ ก่อนถึงแถว legend ท้ายไฟล์)
// → แถวไหน parse ไม่ผ่าน (รหัสผิดรูปแบบ/major ว่าง/สถานภาพไม่ใช่ตัวเลข) ใส่ลง invalid[] พร้อมเหตุผล
// → แถวที่ผ่าน เทียบกับ eligible_students เดิม (query ครั้งเดียวด้วย IN) จัดเป็น new/updated/unchanged
// → ไม่เขียน DB และไม่เก็บ state ฝั่ง server เลย — ส่ง valid[] กลับไปให้ frontend เก็บไว้
// แล้วส่งต่อเป็น body ของ POST /api/admin/eligible-students ตอน admin กด confirm
func (h *AdminController) PreviewEligibleStudents(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		log.Printf("preview eligible students: c.FormFile(\"file\") error: %v | Content-Type: %q | Content-Length: %d",
			err, c.GetHeader("Content-Type"), c.Request.ContentLength)
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "กรุณาแนบไฟล์ .xlsx หรือ .xls ในฟิลด์ file")
		return
	}

	rows, err := readSpreadsheetRows(fileHeader)
	if err != nil {
		log.Printf("preview eligible students: อ่านไฟล์ %q ไม่สำเร็จ: %v", fileHeader.Filename, err)
		utils.Error(c, http.StatusBadRequest, "INVALID_FILE", err.Error())
		return
	}
	if len(rows) < 2 {
		log.Printf("preview eligible students: ไฟล์ %q มี %d แถว (ต้องมีอย่างน้อย header+1 แถว)", fileHeader.Filename, len(rows))
		utils.Error(c, http.StatusBadRequest, "INVALID_FILE", "อ่านข้อมูลในไฟล์ไม่สำเร็จ หรือไม่มีข้อมูล")
		return
	}

	colIdx := map[string]int{}
	for i, header := range rows[0] {
		colIdx[strings.TrimSpace(header)] = i
	}
	studentIDCol, ok1 := colIdx[eligibleExcelHeaders.studentID]
	majorCol, ok2 := colIdx[eligibleExcelHeaders.major]
	statusCol, ok3 := colIdx[eligibleExcelHeaders.status]
	realNameCol, hasRealName := colIdx[eligibleExcelHeaders.realName]
	if !ok1 || !ok2 || !ok3 {
		log.Printf("preview eligible students: หัวตารางที่พบในไฟล์ %q: %v", fileHeader.Filename, rows[0])
		utils.Error(c, http.StatusBadRequest, "INVALID_FILE",
			fmt.Sprintf("หัวตารางในไฟล์ต้องมีคอลัมน์ %q, %q, %q",
				eligibleExcelHeaders.studentID, eligibleExcelHeaders.major, eligibleExcelHeaders.status))
		return
	}

	cell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	valid := make([]dto.EligibleStudentItem, 0, len(rows)-1)
	invalid := make([]dto.InvalidEligibleRow, 0)
	for i, row := range rows[1:] {
		excelRow := i + 2 // +1 เพราะ 0-based, +1 เพราะข้าม header
		studentID := cell(row, studentIDCol)
		if studentID == "" {
			break // ถึงจุดที่ตารางข้อมูลจบแล้ว (ก่อนถึงแถว legend ท้ายไฟล์)
		}

		major := cell(row, majorCol)
		if major == "" {
			invalid = append(invalid, dto.InvalidEligibleRow{Row: excelRow, Reason: "ไม่มีสาขาวิชา"})
			continue
		}
		statusStr := cell(row, statusCol)
		status, err := strconv.Atoi(statusStr)
		if err != nil {
			invalid = append(invalid, dto.InvalidEligibleRow{Row: excelRow, Reason: fmt.Sprintf("สถานภาพ %q ไม่ใช่ตัวเลข", statusStr)})
			continue
		}

		item := dto.EligibleStudentItem{
			StudentID:        studentID,
			Major:            major,
			EnrollmentStatus: status,
		}
		if hasRealName {
			item.RealName = cell(row, realNameCol)
		}
		valid = append(valid, item)
	}

	studentIDs := make([]string, 0, len(valid))
	for _, v := range valid {
		studentIDs = append(studentIDs, v.StudentID)
	}
	var existing []entity.EligibleStudent
	if len(studentIDs) > 0 {
		if err := h.db.WithContext(c.Request.Context()).
			Where("student_id IN ?", studentIDs).Find(&existing).Error; err != nil {
			log.Printf("preview eligible students: query existing error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
	}
	existingByID := make(map[string]entity.EligibleStudent, len(existing))
	for _, e := range existing {
		existingByID[e.StudentID] = e
	}

	summary := dto.EligibleImportSummary{}
	for _, v := range valid {
		old, found := existingByID[v.StudentID]
		switch {
		case !found:
			summary.New++
		case old.Major != v.Major || old.RealName != v.RealName || old.EnrollmentStatus != v.EnrollmentStatus:
			summary.Updated++
		default:
			summary.Unchanged++
		}
	}

	utils.OK(c, http.StatusOK, dto.PreviewEligibleStudentsResponse{
		Valid:   valid,
		Invalid: invalid,
		Summary: summary,
	})
}

// CreateRequestTemplate สร้าง "choice" ใหม่ให้ผู้ใช้เลือก (เช่น small = 500m/512MB)
// data flow: JSON body → bind CreateRequestTemplateRequest → INSERT request_templates (is_active = true)
// → ตอบ template ที่สร้าง → ผู้ใช้จะเห็นทันทีที่ GET /api/request-templates
func (h *AdminController) CreateRequestTemplate(c *gin.Context) {
	var req dto.CreateRequestTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	tmpl := entity.RequestTemplate{
		OptionName:    req.OptionName,
		Category:      req.Category,
		Description:   req.Description,
		RelateSubject: req.RelateSubject,
		CPULimitMilli: req.CPULimitMilli,
		RAMLimitMB:    req.RAMLimitMB,
		StorageGB:     req.StorageGB,
		IsActive:      false,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&tmpl).Error; err != nil {
		utils.Error(c, http.StatusConflict, "TEMPLATE_EXISTS", "ชื่อ template นี้มีอยู่แล้วหรือข้อมูลไม่ถูกต้อง")
		return
	}
	utils.OK(c, http.StatusCreated, tmpl)
}

// UpdateRequestTemplate แก้ไขข้อมูล Template หรือเปิด/ปิดสถานะ (PATCH)
// data flow: อ่าน id จาก path + JSON body → ค้นหาใน DB → อัปเดตข้อมูล → ตอบข้อมูลที่อัปเดตแล้ว
func (h *AdminController) UpdateRequestTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var req dto.UpdateRequestTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// ค้นหา Template เดิมก่อน
	var tmpl entity.RequestTemplate
	if err := h.db.WithContext(c.Request.Context()).First(&tmpl, id).Error; err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบ template นี้ในระบบ")
		return
	}

	// อัปเดตเฉพาะฟิลด์ที่มีการส่งค่ามา (ใช้ Map เพื่อให้รองรับการอัปเดตแบบ Partial หรือบางฟิลด์)
	updates := make(map[string]interface{})

	if req.OptionName != nil {
		updates["name"] = *req.OptionName
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.RelateSubject != nil {
		updates["relate_subject"] = *req.RelateSubject
	}
	if req.CPULimitMilli != nil {
		updates["cpu_limit_milli"] = *req.CPULimitMilli
	}
	if req.RAMLimitMB != nil {
		updates["ram_limit_mb"] = *req.RAMLimitMB
	}
	if req.StorageGB != nil {
		updates["storage_gb"] = *req.StorageGB
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	} // สำคัญมาก สำหรับ Checkbox เปิด/ปิด

	if err := h.db.WithContext(c.Request.Context()).Model(&tmpl).Updates(updates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อัปเดตข้อมูลไม่สำเร็จ")
		return
	}

	// ดึงข้อมูลล่าสุดกลับมาตอบกลับ — อ่านไม่สำเร็จก็ยังตอบ 200 ได้เพราะ UPDATE ผ่านไปแล้วจริง
	// แต่ต้อง log ไว้ ไม่งั้นหน้าเว็บจะได้ค่าเก่ากลับไปแสดงโดยไม่มีใครรู้ว่าทำไม
	if err := h.db.WithContext(c.Request.Context()).First(&tmpl, id).Error; err != nil {
		log.Printf("update request template: re-read id=%d error: %v", id, err)
	}
	utils.OK(c, http.StatusOK, tmpl)
}

func (h *AdminController) ListAllRequestTemplates(c *gin.Context) {
	var templates []entity.RequestTemplate
	// ไม่ต้องใส่ Where("is_active = true") เพื่อดึงมาทั้งหมด
	if err := h.db.WithContext(c.Request.Context()).Order("id").Find(&templates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, templates)
}

// DeleteRequestTemplate ลบ Template ออกจากระบบ (DELETE)
func (h *AdminController) DeleteRequestTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	// ลบข้อมูลจากฐานข้อมูล
	if err := h.db.WithContext(c.Request.Context()).Delete(&entity.RequestTemplate{}, id).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบข้อมูลไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"message": "ลบเทมเพลตสำเร็จ"})
}

// ListNamespaces คืน namespace ทั้งหมดในระบบ พร้อมยอดใช้งานและจำนวนสมาชิก (หน้าภาพรวมของ admin)
// data flow: NamespaceManager.ListAll (SELECT namespaces + SUM ทรัพยากร + COUNT สมาชิกของแต่ละอัน) → ตอบ array
func (h *AdminController) ListNamespaces(c *gin.Context) {
	list, err := h.ns.ListAll(c.Request.Context())
	if err != nil {
		log.Printf("list namespaces error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// SetNamespaceQuota ปรับโควตาของ namespace (เช่น อัปกลุ่มจาก 3 core เป็น 8 core)
//
// data flow: อ่าน id จาก path + JSON body → bind SetQuotaRequest → NamespaceManager.SetQuota
// (ตรวจเพดาน → UPDATE namespaces → sync ResourceQuota ขึ้น cluster) → ตอบ namespace ที่อัปเดตแล้ว
//
// เพดาน: ทุก namespace ไม่เกิน 8 core / 8 GB RAM / 50 GB ดิสก์ เท่ากันหมด
func (h *AdminController) SetNamespaceQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var req dto.SetQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	detail, err := h.ns.SetQuota(c.Request.Context(), id,
		req.CPULimitMilli, req.RAMLimitMB, *req.StorageLimitMB)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrQuotaOutOfRange):
			utils.Error(c, http.StatusBadRequest, "QUOTA_OUT_OF_RANGE", err.Error())
		case errors.Is(err, services.ErrQuotaBelowUsage):
			utils.Error(c, http.StatusConflict, "QUOTA_BELOW_USAGE", err.Error())
		default:
			log.Printf("set quota error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ปรับโควตาไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, detail)
}

func (h *AdminController) ListServices(c *gin.Context) {
	list, err := h.svc.ListAll(c.Request.Context())
	if err != nil {
		log.Printf("admin list services error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงรายการ service ไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

func (h *AdminController) DeleteService(c *gin.Context) {
	id, reason, ok := bindServiceDeletion(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// อ่านข้อมูลสำหรับแจ้งเตือนก่อนลบ — หลังลบแถว service หายไปแล้ว
	notice, err := h.loadServiceNotice(ctx, id)
	if err != nil {
		respondServiceError(c, "admin delete service", err)
		return
	}
	if err := h.svc.DeleteByID(ctx, id); err != nil {
		respondServiceError(c, "admin delete service", err)
		return
	}
	go h.notifyServiceDeletion(context.WithoutCancel(ctx), notice, reason, nil)
	utils.OK(c, http.StatusOK, gin.H{"deleted": id})
}

// ตั้งเวลาลบ service ใน 24 ชม.
func (h *AdminController) ScheduleServiceDelete(c *gin.Context) {
	id, reason, ok := bindServiceDeletion(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	deleteAt, err := h.svc.ScheduleDelete(ctx, id)
	if err != nil {
		respondServiceError(c, "schedule service delete", err)
		return
	}
	if notice, err := h.loadServiceNotice(ctx, id); err != nil {
		log.Printf("schedule service delete id=%d: อ่านข้อมูลสำหรับแจ้งเตือนไม่สำเร็จ: %v", id, err)
	} else {
		go h.notifyServiceDeletion(context.WithoutCancel(ctx), notice, reason, &deleteAt)
	}
	utils.OK(c, http.StatusOK, gin.H{"id": id, "delete_at": deleteAt})
}

func bindServiceDeletion(c *gin.Context) (int, string, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return 0, "", false
	}
	var body dto.DeleteServiceRequest
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "กรุณาระบุเหตุผลในการลบ service")
		return 0, "", false
	}
	return id, strings.TrimSpace(body.Reason), true
}

// serviceNotice = ข้อมูลที่ใช้แจ้งสมาชิกเรื่องการลบ service
type serviceNotice struct {
	serviceName   string
	namespaceName string
	members       []entity.User
}

func (h *AdminController) loadServiceNotice(ctx context.Context, serviceID int) (serviceNotice, error) {
	var svc entity.Service
	if err := h.db.WithContext(ctx).First(&svc, serviceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return serviceNotice{}, services.ErrServiceNotFound
		}
		return serviceNotice{}, err
	}
	var ns entity.Namespace
	if err := h.db.WithContext(ctx).First(&ns, svc.NamespaceID).Error; err != nil {
		return serviceNotice{}, err
	}
	var members []entity.User
	if err := h.db.WithContext(ctx).Where("namespace_id = ?", svc.NamespaceID).Find(&members).Error; err != nil {
		return serviceNotice{}, err
	}
	return serviceNotice{serviceName: svc.Name, namespaceName: ns.Name, members: members}, nil
}

// แจ้งสมาชิกทุกคนใน namespace ว่า service ถูกลบ (deleteAt = nil) หรือถูกตั้งเวลาลบ
func (h *AdminController) notifyServiceDeletion(ctx context.Context, n serviceNotice, reason string, deleteAt *time.Time) {
	if len(n.members) == 0 {
		return
	}
	if !h.mailer.Configured() {
		log.Printf("service '%s': ยังไม่ได้ตั้งค่า SMTP — ไม่ได้ส่งอีเมลแจ้งสมาชิก %d คน", n.serviceName, len(n.members))
		return
	}
	appLink := strings.TrimRight(h.cfg.FrontendOrigin, "/") + "/create-service"
	for _, u := range n.members {
		_ = h.mailer.SendServiceDeletionEmail(ctx, u.ID, u.Gmail, u.RealName, n.serviceName, n.namespaceName, reason, deleteAt, appLink)
	}
}

func (h *AdminController) CancelServiceDelete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}
	if err := h.svc.CancelScheduledDelete(c.Request.Context(), id); err != nil {
		respondServiceError(c, "cancel service delete", err)
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"id": id, "delete_at": nil})
}

func respondServiceError(c *gin.Context, action string, err error) {
	if errors.Is(err, services.ErrServiceNotFound) {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบ service นี้")
		return
	}
	log.Printf("%s error: %v", action, err)
	utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดำเนินการกับ service ไม่สำเร็จ")
}

// DeleteNamespace ลบ namespace ทิ้งทั้งก้อนตามดุลยพินิจแอดมิน — ลบได้แม้ยังมีสมาชิกอยู่
// (ต่างจาก NamespaceController.Leave ที่ผู้ใช้ทั่วไปลบเองไม่ได้ถ้ายังมีสมาชิกคนอื่นอยู่)
//
// data flow: อ่าน id จาก path + JSON body (reason บังคับ) → จดชื่อ namespace และรายชื่อสมาชิกไว้ก่อน
// → NamespaceManager.Delete (ถอนของบนคลัสเตอร์ก่อน แล้วลบแถว → cascade ลบ services/ai_review_requests/
// user_containers ตาม, สมาชิกที่เหลือแค่หลุดออกจาก space) → ส่งอีเมลแจ้งสมาชิกทุกคนพร้อมเหตุผลเบื้องหลัง
//
// ต้องจดรายชื่อก่อนลบ เพราะหลังลบ FK ตั้ง users.namespace_id เป็น NULL แล้วจะไม่รู้อีกว่าใครเคยอยู่ในนั้น
func (h *AdminController) DeleteNamespace(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var body dto.DeleteNamespaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "กรุณาระบุเหตุผลในการลบ namespace")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "กรุณาระบุเหตุผลในการลบ namespace")
		return
	}

	ctx := c.Request.Context()

	var ns entity.Namespace
	if err := h.db.WithContext(ctx).First(&ns, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", services.ErrNamespaceNotFound.Error())
			return
		}
		log.Printf("delete namespace: find namespace error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบ namespace ไม่สำเร็จ")
		return
	}
	var members []entity.User
	if err := h.db.WithContext(ctx).Where("namespace_id = ?", id).Order("id").Find(&members).Error; err != nil {
		log.Printf("delete namespace: load members error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบ namespace ไม่สำเร็จ")
		return
	}

	if err := h.ns.Delete(ctx, id); err != nil {
		switch {
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		default:
			log.Printf("delete namespace error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบ namespace ไม่สำเร็จ")
		}
		return
	}

	// ส่งเบื้องหลังไม่ให้แอดมินรอ SMTP ทีละฉบับ (ฉบับละได้ถึง 15 วิ) — namespace ถูกลบไปแล้วจริง
	// ส่งไม่ออกก็ไม่ย้อนการลบ ผลทุกฉบับถูก MailJournal บันทึกลง email_deliveries ให้ตามดูได้
	go h.notifyNamespaceDeleted(context.WithoutCancel(ctx), ns.Name, members, reason)

	utils.OK(c, http.StatusOK, gin.H{"deleted": id, "notified": len(members)})
}

// notifyNamespaceDeleted ส่งอีเมลแจ้งสมาชิกทีละคน — คนหนึ่งส่งไม่ผ่านไม่กระทบคนถัดไป
// ผลการส่ง (รวมกรณีล้มเหลว) ถูก MailJournal บันทึกและ log ให้แล้ว จึงไม่ต้องจัดการ error ซ้ำที่นี่
func (h *AdminController) notifyNamespaceDeleted(ctx context.Context, nsName string, members []entity.User, reason string) {
	if len(members) == 0 {
		return
	}
	if !h.mailer.Configured() {
		log.Printf("delete namespace '%s': ยังไม่ได้ตั้งค่า SMTP — ไม่ได้ส่งอีเมลแจ้งสมาชิก %d คน", nsName, len(members))
		return
	}
	appLink := strings.TrimRight(h.cfg.FrontendOrigin, "/") + "/"
	for _, u := range members {
		_ = h.mailer.SendNamespaceDeletedEmail(ctx, u.ID, u.Gmail, u.RealName, nsName, reason, appLink)
	}
}

// ListAllRequests คืนคำขอ VM/namespace ทั้งหมดในระบบ (ทุกสถานะ) พร้อมชื่อ/รหัส นศ. ของผู้ยื่น ให้ admin ดู
//
// data flow: SELECT requests ทั้งหมด → enrichRequests เติมชื่อ/รหัส นศ. ของผู้ยื่นให้
// (ถาม users เป็นก้อนเดียว กัน N+1 — ดู admin_dashboard.go)
func (h *AdminController) ListAllRequests(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []entity.Request
	if err := h.db.WithContext(ctx).Order("created_at DESC").Find(&requests).Error; err != nil {
		log.Printf("list requests error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}

	out, err := h.enrichRequests(c, requests)
	if err != nil {
		log.Printf("list requests: load requesters error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, out)
}

// Approve อนุมัติคำขอ → สร้าง namespace จริงให้ผู้ยื่น (ใช้ NamespaceManager.Create ตัวเดียวกับที่
// ผู้ใช้สร้าง space เองใช้ — ได้ทั้งการเช็ค NOT NULL/unique ของคอลัมน์, ผูก users.namespace_id,
// และเรียก Provisioner.EnsureNamespace ครบในที่เดียว ไม่ hand-roll insert เองอีก)
//
// data flow: ล็อกแถว requests ด้วย FOR UPDATE (กันแอดมิน 2 คนกด approve พร้อมกัน) → เช็คว่ายัง pending
// → ปล่อยล็อก (ปิด transaction) → เรียก NamespaceManager.Create แยกนอก transaction เพราะมันคุยกับ
// cluster จริงข้างใน (ไม่อยากถือ DB transaction ค้างไว้ระหว่างรอ network) → สำเร็จค่อย mark approved
func (h *AdminController) Approve(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}
	ctx := c.Request.Context()

	var req entity.Request
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&req, id).Error; err != nil {
			return err
		}
		if req.Status != entity.RequestPending {
			return errRequestNotPending
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบคำขอนี้")
		case errors.Is(err, errRequestNotPending):
			utils.Error(c, http.StatusConflict, "NOT_PENDING", "คำขอนี้ถูกดำเนินการไปแล้ว")
		default:
			log.Printf("approve request lock error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		}
		return
	}

	// โควตาของ space มาจากที่ผู้ใช้กรอกไว้ตอนยื่นคำขอ ไม่ใช่ค่าตั้งต้นของระบบ — คำขอทั้งใบ
	// มีไว้เพื่อให้แอดมินตัดสินตัวเลขคู่นี้โดยเฉพาะ ถ้าอนุมัติแล้วยังได้ค่าตั้งต้นเท่ากันหมด
	// การกรอก cpu/ram ในหน้ายื่นคำขอก็ไม่มีความหมาย และแอดมินต้องตามไปปรับโควตาให้ทีหลังทุกใบ
	name := fmt.Sprintf("ns-user-%d", req.UserID)
	ns, err := h.ns.Create(ctx, req.UserID, name, req.CPULimitMilli, req.RAMLimitMB)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNameTaken):
			utils.Error(c, http.StatusConflict, "NAME_TAKEN", err.Error())
		case errors.Is(err, services.ErrQuotaOutOfRange):
			// คำขอเก่าที่ยื่นไว้ก่อนมีการเช็คเพดานตอนยื่น (หรือเพดานถูกปรับลงทีหลัง)
			// บอกให้ชัดว่าติดที่ตัวเลขในคำขอ ไม่ใช่ระบบพัง — แอดมินจะได้ deny แล้วให้ยื่นใหม่
			utils.Error(c, http.StatusUnprocessableEntity, "QUOTA_OUT_OF_RANGE",
				"โควตาที่ระบุในคำขอเกินเพดานที่ระบบอนุญาต: "+err.Error())
		case errors.Is(err, services.ErrNamespaceTerminating):
			// เจอตอนแอดมินลบ space ของคนนี้แล้วรีบกด approve คำขอใหม่ให้เขาทันที
			// ชื่อ namespace ที่ Approve ตั้งเป็น ns-user-<id> ตายตัว จึงชนกับตัวเดิมที่ยังไม่ตายสนิท
			utils.Error(c, http.StatusConflict, "NAMESPACE_TERMINATING", err.Error())
		default:
			log.Printf("approve: provision namespace error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง namespace ไม่สำเร็จ")
		}
		return
	}

	req.Status = entity.RequestApproved
	if err := h.db.WithContext(ctx).Save(&req).Error; err != nil {
		log.Printf("approve: namespace created (id=%d) but failed to mark request approved: %v", ns.ID, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง namespace สำเร็จแต่บันทึกสถานะคำขอไม่สำเร็จ กรุณาตรวจสอบ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"request": req, "namespace": ns})
}

// Deny ปฏิเสธคำขอ — พลิกสถานะเป็น denied พร้อมเก็บเหตุผลที่ admin เขียน
//
// data flow: อ่าน id จาก path + JSON body → bind DenyRequestRequest (reason บังคับ)
// → ล็อกแถว requests ด้วย FOR UPDATE (กันชนกับ Approve/Deny ที่วิ่งพร้อมกัน) → เช็คว่ายัง pending
// → บันทึก status = denied + deny_reason ในทรานแซกชันเดียว
//
// โครงเดียวกับ Approve ด้านบน — เหตุผลถูกเก็บไว้กับคำขอ ผู้ยื่นอ่านได้จากหน้า "คำขอของฉัน"
func (h *AdminController) Deny(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var body dto.DenyRequestRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "กรุณาระบุเหตุผลในการปฏิเสธคำขอ")
		return
	}

	ctx := c.Request.Context()

	var req entity.Request
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&req, id).Error; err != nil {
			return err
		}
		if req.Status != entity.RequestPending {
			return errRequestNotPending
		}
		req.Status = entity.RequestDenied
		req.DenyReason = reason
		return tx.Save(&req).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบคำขอนี้")
		case errors.Is(err, errRequestNotPending):
			utils.Error(c, http.StatusConflict, "NOT_PENDING", "คำขอนี้ถูกดำเนินการไปแล้ว")
		default:
			log.Printf("deny request error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		}
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"id": id, "status": entity.RequestDenied, "deny_reason": reason})
}

// ListUsers คืนผู้ใช้งานทั้งหมด พร้อมโควตา CPU/RAM ของ namespace ที่ผู้ใช้สังกัด
// (โควตาผูกกับ namespace ไม่ใช่ user แล้ว)
//
// data flow: SELECT users → รวม namespace_id ที่พบมาถามเป็นก้อนเดียว (กัน N+1) → ห่อเป็น
// dto.UserWithNamespace ตอบกลับ (ผู้ใช้ที่ยังไม่มี space → โควตา 0)
func (h *AdminController) ListUsers(c *gin.Context) {
	ctx := c.Request.Context()

	var users []entity.User
	if err := h.db.WithContext(ctx).Order("id").Find(&users).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลผู้ใช้งานไม่สำเร็จ")
		return
	}

	// โหลดโควตาของทุก namespace ที่ผู้ใช้กลุ่มนี้สังกัดในทีเดียว (กัน N+1 query)
	nsIDs := make([]int, 0, len(users))
	for _, u := range users {
		if u.NamespaceID != nil {
			nsIDs = append(nsIDs, *u.NamespaceID)
		}
	}
	var namespaces []entity.Namespace
	if len(nsIDs) > 0 {
		if err := h.db.WithContext(ctx).Where("id IN ?", nsIDs).Find(&namespaces).Error; err != nil {
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลผู้ใช้งานไม่สำเร็จ")
			return
		}
	}
	nsByID := make(map[int]entity.Namespace, len(namespaces))
	for _, ns := range namespaces {
		nsByID[ns.ID] = ns
	}

	out := make([]dto.UserWithNamespace, 0, len(users))
	for _, u := range users {
		view := dto.UserWithNamespace{User: u}
		if u.NamespaceID != nil {
			if ns, ok := nsByID[*u.NamespaceID]; ok {
				view.NamespaceName = ns.Name
				view.CPULimitMilli = ns.CPULimitMilli
				view.RAMLimitMB = ns.RAMLimitMB
			}
		}
		out = append(out, view)
	}
	utils.OK(c, http.StatusOK, out)
}

// UpdateUser แก้ไขข้อมูลผู้ใช้งาน (PATCH)
// data flow: อ่าน id → ตรวจสอบ User → เตรียมข้อมูล Map อัปเดต → UPDATE users
func (h *AdminController) UpdateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	ctx := c.Request.Context()
	var user entity.User
	if err := h.db.WithContext(ctx).First(&user, id).Error; err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบผู้ใช้งานในระบบ")
		return
	}

	if req.RoleID != nil && *req.RoleID != user.RoleID && id == c.GetInt("userID") {
		utils.Error(c, http.StatusBadRequest, "CANNOT_CHANGE_OWN_ROLE", "ไม่สามารถแก้ไข Role ของตัวเองได้")
		return
	}

	updates := make(map[string]interface{})

	if req.StudentID != nil {
		// ถ้ามีการเปลี่ยนรหัสนักศึกษา ต้องยัดลง eligible_students กันติด FK ก่อน
		//
		// เช็ค error ตรงนี้เพื่อให้ข้อความที่ผู้ใช้เห็นตรงกับสาเหตุจริง: ถ้าปล่อยผ่าน แถวนี้จะไม่ถูก
		// สร้าง แล้วไปพังที่ Updates ข้างล่างด้วย FK violation ซึ่งตอบกลับเป็น "อัปเดตข้อมูลผู้ใช้งาน
		// ไม่สำเร็จ" — แอดมินจะไล่หาสาเหตุผิดจุดเพราะปัญหาจริงอยู่คนละตารางกัน
		eligible := entity.EligibleStudent{StudentID: *req.StudentID, Major: "Updated by Admin"}
		if err := h.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
			Create(&eligible).Error; err != nil {
			log.Printf("update user: ensure eligible student %q error: %v", *req.StudentID, err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL",
				"เพิ่มรหัสประจำตัวใหม่เข้ารายชื่อที่อนุญาตไม่สำเร็จ")
			return
		}
		updates["student_id"] = *req.StudentID
	}
	if req.RealName != nil {
		updates["real_name"] = *req.RealName
	}
	if req.Gmail != nil {
		updates["gmail"] = *req.Gmail
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}

	if err := h.db.WithContext(ctx).Model(&user).Updates(updates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อัปเดตข้อมูลผู้ใช้งานไม่สำเร็จ")
		return
	}

	// ดึงข้อมูลล่าสุดมาตอบ — เหตุผลเดียวกับ UpdateRequestTemplate: UPDATE ผ่านไปแล้ว
	// อ่านกลับไม่ได้ไม่ถือว่าคำสั่งล้มเหลว แต่ต้องมีร่องรอยว่าค่าที่ตอบกลับอาจไม่ใช่ค่าล่าสุด
	if err := h.db.WithContext(ctx).First(&user, id).Error; err != nil {
		log.Printf("update user: re-read id=%d error: %v", id, err)
	}

	// role ของบัญชีเปลี่ยน → ซิงก์ role ในรายชื่อผู้มีสิทธิ์ตาม ให้หน้า "รายชื่อผู้มีสิทธิ์" ตรงกับบัญชีจริง
	// ซิงก์ไม่ได้ไม่ถือว่าล้มเหลว — บัญชีเปลี่ยนแล้วจริง และจะถูกซิงก์ซ้ำตอน server start (config.syncEligibleRoles)
	if req.RoleID != nil {
		if err := h.db.WithContext(ctx).Exec(
			`UPDATE eligible_students SET role = r.name FROM roles r WHERE r.id = ? AND eligible_students.student_id = ?`,
			user.RoleID, user.StudentID).Error; err != nil {
			log.Printf("update user: sync eligible role for %q error: %v", user.StudentID, err)
		}
	}

	// ตอบพร้อมโควตาของ namespace เหมือน ListUsers — ไม่งั้นแถวนี้ในตารางฝั่ง frontend
	// จะเห็น quota หายไปทันทีหลังบันทึก (ถูกแทนที่ด้วย response ที่ไม่มีฟิลด์นี้)
	view := dto.UserWithNamespace{User: user}
	if user.NamespaceID != nil {
		var ns entity.Namespace
		if err := h.db.WithContext(ctx).First(&ns, *user.NamespaceID).Error; err == nil {
			view.CPULimitMilli = ns.CPULimitMilli
			view.RAMLimitMB = ns.RAMLimitMB
		}
	}
	utils.OK(c, http.StatusOK, view)
}

// DeleteUser ลบผู้ใช้งาน (DELETE)
//
// ต้องถอนของบนคลัสเตอร์ให้หมดก่อนค่อยลบแถว user เพราะ FK ทำงานเงียบเกินไป:
//   - fk_namespaces_contributor_id (ON DELETE CASCADE) → ลบเจ้าของ = แถว namespace หายตามทันที
//   - fk_services_created_by (ON DELETE CASCADE) → service ที่คนนี้สร้างไว้ใน space คนอื่นก็หายตาม
//
// ถ้าปล่อยให้ FK จัดการอย่างเดียว record ใน DB จะหายไปโดยที่ namespace/workload จริงบนคลัสเตอร์
// ยังรันอยู่ กลายเป็นขยะที่ไม่มีทางตามเก็บได้อีกเลย (ไม่เหลือชื่อ/id ให้ค้น) แถมโควตาที่ระบบเรา
// คำนวณจาก DB จะว่างขึ้นทั้งที่ของจริงยังกินทรัพยากรอยู่
//
// data flow: หา namespace ที่ user เป็นเจ้าของ → NamespaceManager.Delete ทีละก้อน (ลบบนคลัสเตอร์
// + cascade service ข้างใน) → เก็บ service ที่เหลือของ user ใน space คนอื่นผ่าน ServiceManager.Delete
// → ค่อยลบแถว user
func (h *AdminController) DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if msg, err := h.deleteUserAccount(c.Request.Context(), id); err != nil {
		log.Printf("delete user %d: %v", id, err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", msg)
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"message": "ลบผู้ใช้งานสำเร็จ"})
}

func (h *AdminController) deleteUserAccount(ctx context.Context, id int) (string, error) {
	const failed = "ลบผู้ใช้งานไม่สำเร็จ"

	// 1) space ที่ user เป็นเจ้าของ — ลบทั้งก้อน (service/ใบเสร็จ AI/แถว monitoring ข้างในหายตาม
	//    ด้วย FK ส่วนสมาชิกคนอื่นแค่หลุดออกจาก space ไม่ถูกลบบัญชี)
	var owned []entity.Namespace
	if err := h.db.WithContext(ctx).Where("contributor_id = ?", id).Find(&owned).Error; err != nil {
		return failed, fmt.Errorf("list owned namespaces: %w", err)
	}
	for _, ns := range owned {
		if err := h.ns.Delete(ctx, ns.ID); err != nil && !errors.Is(err, services.ErrNamespaceNotFound) {
			return "ลบ namespace ของผู้ใช้ไม่สำเร็จ จึงยังไม่ได้ลบผู้ใช้ (ลองใหม่อีกครั้ง)",
				fmt.Errorf("delete namespace %d: %w", ns.ID, err)
		}
	}

	// 2) service ที่ user ไปสร้างค้างไว้ใน space ของคนอื่น — ถอนทีละตัวให้โควตาของกลุ่มนั้นคืนจริง
	var leftovers []entity.Service
	if err := h.db.WithContext(ctx).Where("created_by = ?", id).Order("id").Find(&leftovers).Error; err != nil {
		return failed, fmt.Errorf("list leftover services: %w", err)
	}
	for _, svc := range leftovers {
		if err := h.svc.Delete(ctx, svc.ID, svc.NamespaceID); err != nil && !errors.Is(err, services.ErrServiceNotFound) {
			return "ลบ service ของผู้ใช้ไม่สำเร็จ จึงยังไม่ได้ลบผู้ใช้ (ลองใหม่อีกครั้ง)",
				fmt.Errorf("delete service %d: %w", svc.ID, err)
		}
	}

	if err := h.db.WithContext(ctx).Delete(&entity.User{}, id).Error; err != nil {
		return failed, fmt.Errorf("delete user row: %w", err)
	}
	return "", nil
}
