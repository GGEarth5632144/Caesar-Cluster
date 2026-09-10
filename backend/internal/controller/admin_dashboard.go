package controller

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/utils"
)

// dashboardTimelineDays = จำนวนวันย้อนหลังของกราฟแท่ง "คำขอใหม่รายวัน" บนหน้า AdminDashboard
const dashboardTimelineDays = 7

// DashboardSummary = ทุกอย่างที่หน้า AdminDashboard ต้องใช้จากฝั่ง DB ในก้อนเดียว
//
// เดิมหน้านั้นดึง GET /admin/users กับ GET /admin/requests มาทั้งตาราง แล้วเอามานับเองในเบราว์เซอร์
// ทั้งที่ใช้จริงแค่ "จำนวน" ผู้ใช้ กับ "จำนวน" คำขอแยกตามสถานะ — รายชื่อผู้ใช้ทุกคนพร้อม gmail/
// โควตา และคำขอที่ปิดจบไปแล้วทุกใบ เดินทางข้ามเน็ตมาเพื่อให้ .length กับ .filter() เท่านั้น
// แล้วหน้านั้นยัง refresh ตัวเองทุก 30 วินาที ค่าใช้จ่ายนี้จึงเกิดซ้ำตลอดเวลาที่แท็บเปิดค้างไว้
//
// เหลือแต่ PendingRequests ที่ต้องส่งเป็นแถวเต็ม เพราะการ์ด "Action Required" โชว์ชื่อผู้ยื่น
// กับสเปกที่ขอจริงๆ — และเป็นชุดที่ไม่โตตามเวลา (อนุมัติแล้วก็หลุดออกจากลิสต์)
type DashboardSummary struct {
	UserCount       int64                      `json:"user_count"`
	RequestCounts   DashboardRequestCounts     `json:"request_counts"`
	RequestTimeline []DashboardTimelinePoint   `json:"request_timeline"`
	PendingRequests []dto.RequestWithRequester `json:"pending_requests"`
}

// DashboardRequestCounts = จำนวนคำขอแยกตามสถานะ (ที่มาของกราฟวงกลมและตัวเลขสรุปด้านบน)
type DashboardRequestCounts struct {
	Pending  int64 `json:"pending"`
	Approved int64 `json:"approved"`
	Denied   int64 `json:"denied"`
	Total    int64 `json:"total"`
}

// DashboardTimelinePoint = คำขอที่ยื่นเข้ามาในหนึ่งวัน (Date เป็น YYYY-MM-DD ตามเขตเวลาของผู้ดู)
type DashboardTimelinePoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// tzOffsetMinutes อ่านเขตเวลาของเบราว์เซอร์จาก query ?tz_offset= (หน่วยนาที "เร็วกว่า UTC")
//
// ต้องรู้ค่านี้เพราะ requests.created_at เก็บเป็นเวลา UTC แต่กราฟต้องแบ่งวันตามเวลาที่ผู้ดูเห็น
// ไม่งั้นคำขอที่ยื่นตอนเช้ามืดตามเวลาไทยจะไปตกอยู่ในแท่งของ "เมื่อวาน"
//
// ส่งเป็นตัวเลข offset แทนชื่อโซน (Asia/Bangkok) โดยตั้งใจ — ชื่อโซนต้องพึ่งฐานข้อมูล tzdata
// ที่ Go บน Windows ไม่ได้ติดมาให้ ส่วน offset เป็นเลขจำนวนเต็มที่คำนวณตรงๆ ได้ทุกที่
// default 420 = UTC+7 (เวลาไทย) สำหรับผู้เรียกที่ไม่ได้ส่งค่ามา
func tzOffsetMinutes(c *gin.Context) int {
	raw := c.Query("tz_offset")
	if raw == "" {
		return 420
	}
	mins, err := strconv.Atoi(raw)
	// ±14 ชั่วโมงคือช่วงที่เขตเวลาจริงบนโลกเป็นไปได้ นอกจากนี้ถือว่าค่าเพี้ยน ใช้ default แทน
	if err != nil || mins < -14*60 || mins > 14*60 {
		return 420
	}
	return mins
}

// enrichRequests เติมชื่อ/รหัส นศ. ของผู้ยื่นให้คำขอแต่ละแถว โดยถาม users เป็นก้อนเดียว (กัน N+1)
// ใช้ร่วมกันระหว่าง ListAllRequests กับ DashboardSummary — ทั้งสองที่ต้องการรูปแบบเดียวกันเป๊ะ
func (h *AdminController) enrichRequests(c *gin.Context, requests []entity.Request) ([]dto.RequestWithRequester, error) {
	out := make([]dto.RequestWithRequester, 0, len(requests))
	if len(requests) == 0 {
		return out, nil
	}

	seen := make(map[int]bool, len(requests))
	userIDs := make([]int, 0, len(requests))
	for _, r := range requests {
		if !seen[r.UserID] {
			seen[r.UserID] = true
			userIDs = append(userIDs, r.UserID)
		}
	}

	var users []entity.User
	if err := h.db.WithContext(c.Request.Context()).
		Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]entity.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	for _, r := range requests {
		view := dto.RequestWithRequester{Request: r}
		// หา user ไม่เจอ (ถูกลบไปแล้ว) ปล่อยชื่อว่างไว้ ไม่ error ทั้งก้อน
		if u, ok := byID[r.UserID]; ok {
			view.RequesterName = u.RealName
			view.RequesterStudentID = u.StudentID
		}
		out = append(out, view)
	}
	return out, nil
}

// DashboardSummary ตอบข้อมูลสรุปของหน้า AdminDashboard ในคำขอเดียว
//
// data flow: COUNT users → GROUP BY status ที่ requests → GROUP BY วันที่ (7 วันหลังสุด)
// → SELECT เฉพาะคำขอที่ยัง pending แล้ว enrich ชื่อผู้ยื่น → ห่อรวมส่งกลับก้อนเดียว
//
// การนับทั้งหมดเกิดที่ Postgres ซึ่งอ่าน index ได้โดยไม่ต้องยกทุกแถวขึ้นมาบนแอป และผลลัพธ์
// ที่วิ่งกลับไปคือตัวเลขไม่กี่ตัว แทนที่จะเป็นสองตารางเต็ม
func (h *AdminController) DashboardSummary(c *gin.Context) {
	ctx := c.Request.Context()
	offset := tzOffsetMinutes(c)

	var summary DashboardSummary

	if err := h.db.WithContext(ctx).Model(&entity.User{}).Count(&summary.UserCount).Error; err != nil {
		log.Printf("dashboard summary: count users error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลสรุปไม่สำเร็จ")
		return
	}

	var statusRows []struct {
		Status string
		N      int64
	}
	if err := h.db.WithContext(ctx).Table("requests").
		Select("status, COUNT(*) AS n").Group("status").Scan(&statusRows).Error; err != nil {
		log.Printf("dashboard summary: count requests error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลสรุปไม่สำเร็จ")
		return
	}
	for _, row := range statusRows {
		switch row.Status {
		case entity.RequestPending:
			summary.RequestCounts.Pending = row.N
		case entity.RequestApproved:
			summary.RequestCounts.Approved = row.N
		case entity.RequestDenied:
			summary.RequestCounts.Denied = row.N
		}
		summary.RequestCounts.Total += row.N
	}

	// ขอบล่างของกราฟ = เที่ยงคืนของวันแรกในหน้าต่าง ตามเวลาของผู้ดู แล้วแปลงกลับเป็น UTC
	// เพื่อเทียบกับ created_at ที่เก็บเป็น UTC
	shift := time.Duration(offset) * time.Minute
	viewerNow := time.Now().UTC().Add(shift)
	firstDay := time.Date(viewerNow.Year(), viewerNow.Month(), viewerNow.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(dashboardTimelineDays - 1))
	windowStartUTC := firstDay.Add(-shift)

	var dayRows []struct {
		Date string
		N    int64
	}
	// Raw SQL ด้วยเหตุผลเดียวกับ PowerService.GetPowerHistory — Group() ของ GORM
	// จะ quote "1" เป็นชื่อคอลัมน์ ทำให้ GROUP BY ตามลำดับคอลัมน์ใช้ไม่ได้
	if err := h.db.WithContext(ctx).Raw(`
		SELECT to_char(date_trunc('day', created_at + make_interval(mins => ?)), 'YYYY-MM-DD') AS date,
		       COUNT(*) AS n
		FROM requests
		WHERE created_at >= ?
		GROUP BY 1
		ORDER BY 1`, offset, windowStartUTC).Scan(&dayRows).Error; err != nil {
		log.Printf("dashboard summary: request timeline error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลสรุปไม่สำเร็จ")
		return
	}
	countByDate := make(map[string]int64, len(dayRows))
	for _, row := range dayRows {
		countByDate[row.Date] = row.N
	}
	// เติมวันที่ไม่มีคำขอเลยให้เป็น 0 — กราฟแท่งจะได้มีแกน X ครบ 7 วันเท่ากันทุกครั้ง
	// ไม่ใช่หดๆ ขยายๆ ตามว่าวันไหนบังเอิญมีคนยื่น
	summary.RequestTimeline = make([]DashboardTimelinePoint, 0, dashboardTimelineDays)
	for i := 0; i < dashboardTimelineDays; i++ {
		date := firstDay.AddDate(0, 0, i).Format("2006-01-02")
		summary.RequestTimeline = append(summary.RequestTimeline, DashboardTimelinePoint{
			Date:  date,
			Count: countByDate[date],
		})
	}

	var pending []entity.Request
	if err := h.db.WithContext(ctx).
		Where("status = ?", entity.RequestPending).
		Order("created_at DESC").Find(&pending).Error; err != nil {
		log.Printf("dashboard summary: list pending error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลสรุปไม่สำเร็จ")
		return
	}
	enriched, err := h.enrichRequests(c, pending)
	if err != nil {
		log.Printf("dashboard summary: load requesters error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลสรุปไม่สำเร็จ")
		return
	}
	summary.PendingRequests = enriched

	utils.OK(c, http.StatusOK, summary)
}
