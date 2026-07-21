package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/extrame/xls"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/services"
	"backend/internal/utils"
)

// errRequestNotPending = internal sentinel ใช้เทียบใน Approve เมื่อคำขอถูกจัดการไปแล้ว
var errRequestNotPending = errors.New("request ถูกดำเนินการไปแล้ว")

// AdminController รวม endpoint ฝั่ง admin ไว้ที่เดียว:
// import รายชื่อ นศ. ที่มีสิทธิ์, สร้าง choices (plans), ดูภาพรวม namespace, ปรับโควตาให้กลุ่ม
// ทุก route ที่ผูกกับ controller นี้ผ่าน middleware AdminOnly มาแล้ว
type AdminController struct {
	db *gorm.DB
	ns *services.NamespaceManager
}

// NewAdminController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewAdminController(db *gorm.DB, ns *services.NamespaceManager) *AdminController {
	return &AdminController{db: db, ns: ns}
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

// studentIDPattern = รูปแบบรหัสนักศึกษาที่ยอมรับตอน parse ไฟล์ Excel: ตัวอักษรนำ 1 ตัว + ตัวเลขอย่างน้อย 6 หลัก
// (เช่น "B6600907") กันไม่ให้แถว header/legend ที่หลุดเข้ามาโดนนับเป็นแถวข้อมูล
var studentIDPattern = regexp.MustCompile(`^[A-Za-z][0-9]{6,}$`)

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

		if !studentIDPattern.MatchString(studentID) {
			invalid = append(invalid, dto.InvalidEligibleRow{Row: excelRow, Reason: "รูปแบบรหัสประจำตัวไม่ถูกต้อง"})
			continue
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

	// ดึงข้อมูลล่าสุดกลับมาตอบกลับ
	h.db.First(&tmpl, id)
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
// (ตรวจเพดานตามชนิด space → UPDATE namespaces → sync ResourceQuota ขึ้น cluster) → ตอบ namespace ที่อัปเดตแล้ว
//
// เพดาน: กลุ่มไม่เกิน 8 core / 8 GB, เดี่ยวไม่เกินค่า default (3 core / 2 GB)
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

	detail, err := h.ns.SetQuota(c.Request.Context(), id, req.CPULimitMilli, req.RAMLimitMB, req.MaxServices)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrQuotaOutOfRange):
			utils.Error(c, http.StatusBadRequest, "QUOTA_OUT_OF_RANGE", err.Error())
		default:
			log.Printf("set quota error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ปรับโควตาไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, detail)
}

// ListAllRequests คืนคำขอ VM/namespace ทั้งหมดในระบบ (ทุกสถานะ) พร้อมชื่อ/รหัส นศ. ของผู้ยื่น ให้ admin ดู
//
// data flow: SELECT requests ทั้งหมด → เก็บ user_id ที่พบมาถามเป็นก้อนเดียว (กัน N+1 query)
// → จับคู่กลับเป็น RequestWithRequester ทีละแถว (ตัวไหนหา user ไม่เจอ ปล่อยชื่อว่างไว้ ไม่ error ทั้งก้อน)
func (h *AdminController) ListAllRequests(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []entity.Request
	if err := h.db.WithContext(ctx).Order("created_at DESC").Find(&requests).Error; err != nil {
		log.Printf("list requests error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}

	userIDs := make([]int, 0, len(requests))
	for _, r := range requests {
		userIDs = append(userIDs, r.UserID)
	}
	var users []entity.User
	if len(userIDs) > 0 {
		if err := h.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			log.Printf("list requests: load requesters error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
			return
		}
	}
	byID := make(map[int]entity.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	out := make([]dto.RequestWithRequester, 0, len(requests))
	for _, r := range requests {
		view := dto.RequestWithRequester{Request: r}
		if u, ok := byID[r.UserID]; ok {
			view.RequesterName = u.RealName
			view.RequesterStudentID = u.StudentID
		}
		out = append(out, view)
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

	name := fmt.Sprintf("ns-user-%d", req.UserID)
	ns, err := h.ns.Create(ctx, req.UserID, name, req.NamespaceName)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNameTaken):
			utils.Error(c, http.StatusConflict, "NAME_TAKEN", err.Error())
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

// Deny ปฏิเสธคำขอ — แค่พลิกสถานะแบบ atomic (WHERE status = pending กันชนกับ Approve ที่วิ่งพร้อมกัน)
func (h *AdminController) Deny(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	res := h.db.WithContext(c.Request.Context()).
		Model(&entity.Request{}).
		Where("id = ? AND status = ?", id, entity.RequestPending).
		Update("status", entity.RequestDenied)
	if res.Error != nil {
		log.Printf("deny request error: %v", res.Error)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	if res.RowsAffected == 0 {
		utils.Error(c, http.StatusConflict, "NOT_PENDING", "ไม่พบคำขอนี้ หรือถูกดำเนินการไปแล้ว")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"id": id, "status": entity.RequestDenied})
}

// ListUsers คืนผู้ใช้งานทั้งหมด พร้อมชั้นปีที่คำนวณสดจาก student_id (ไม่ใช่ค่า User.EntryYear ที่เก็บดิบๆ)
// data flow: SELECT users → คำนวณ YearLevel ทีละคนจาก student_id (แกะไม่ได้ เช่น account admin → 0)
// → ห่อเป็น dto.UserWithYearLevel ตอบกลับ ให้หน้า User Management โชว์ "Year 4" ได้ตรงๆ
func (h *AdminController) ListUsers(c *gin.Context) {
	var users []entity.User
	if err := h.db.WithContext(c.Request.Context()).Order("id").Find(&users).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลผู้ใช้งานไม่สำเร็จ")
		return
	}

	now := time.Now()
	out := make([]dto.UserWithYearLevel, 0, len(users))
	for _, u := range users {
		yearLevel, err := entity.YearLevel(u.StudentID, now)
		if err != nil {
			yearLevel = 0 // เช่น account admin ที่ student_id ไม่ตรงรูปแบบ นศ. — โชว์ 0 แทนพัง
		}
		out = append(out, dto.UserWithYearLevel{User: u, YearLevel: yearLevel})
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

	updates := make(map[string]interface{})

	if req.StudentID != nil {
		// ถ้ามีการเปลี่ยนรหัสนักศึกษา ต้องยัดลง eligible_students กันติด FK ก่อน
		eligible := entity.EligibleStudent{StudentID: *req.StudentID, Major: "Updated by Admin"}
		h.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&eligible)
		updates["student_id"] = *req.StudentID
	}
	if req.RealName != nil {
		updates["real_name"] = *req.RealName
	}
	if req.Gmail != nil {
		updates["gmail"] = *req.Gmail
	}
	if req.NickName != nil {
		updates["nick_name"] = *req.NickName
	}
	if req.Year != nil {
		updates["year"] = *req.Year
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}
	if req.CPUlimit != nil {
		updates["cpu_limit"] = *req.CPUlimit
	}
	if req.Ramlimit != nil {
		updates["ram_limit"] = *req.Ramlimit
	}

	if err := h.db.WithContext(ctx).Model(&user).Updates(updates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อัปเดตข้อมูลผู้ใช้งานไม่สำเร็จ")
		return
	}

	h.db.First(&user, id) // ดึงข้อมูลล่าสุดมาตอบ

	// ตอบพร้อม year_level เหมือน ListUsers — ไม่งั้นแถวนี้ในตารางฝั่ง frontend จะเห็น year_level
	// หายไปทันทีหลังบันทึก (ถูกแทนที่ด้วย response ที่ไม่มีฟิลด์นี้)
	yearLevel, err := entity.YearLevel(user.StudentID, time.Now())
	if err != nil {
		yearLevel = 0
	}
	utils.OK(c, http.StatusOK, dto.UserWithYearLevel{User: user, YearLevel: yearLevel})
}

// DeleteUser ลบผู้ใช้งาน (DELETE)
func (h *AdminController) DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&entity.User{}, id).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบผู้ใช้งานไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"message": "ลบผู้ใช้งานสำเร็จ"})
}