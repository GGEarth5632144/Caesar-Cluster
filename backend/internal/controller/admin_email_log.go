package controller

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"backend/internal/entity"
	"backend/internal/utils"
)

// จำนวนแถวต่อหน้าของ /api/admin/email-deliveries
const (
	emailLogDefaultLimit = 50
	emailLogMaxLimit     = 200
)

// ListEmailDeliveries คืนประวัติการส่งอีเมลของระบบ (admin only) กรองด้วย status/purpose/gmail/limit
// มีไว้ตอบว่า "นักศึกษาบอกว่าไม่ได้รับอีเมล — ระบบส่งไปจริงไหม"
//
// status = "sent" แปลว่า SMTP ตอบรับแล้วเท่านั้น ไม่ได้แปลว่าเข้ากล่องขาเข้า (ให้ไปหาในเมลขยะก่อน)
// ส่วน "failed" มี error เต็มๆ ให้อ่านว่าพังตรงไหน
func (h *AdminController) ListEmailDeliveries(c *gin.Context) {
	db := h.db.WithContext(c.Request.Context())

	q := db.Model(&entity.EmailDelivery{})
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if purpose := c.Query("purpose"); purpose != "" {
		q = q.Where("purpose = ?", purpose)
	}
	// ค้นด้วยอีเมลปลายทางคือทางที่ admin ใช้จริงที่สุด (นักศึกษาแจ้งมาพร้อมอีเมลของตัวเอง)
	if gmail := strings.TrimSpace(c.Query("gmail")); gmail != "" {
		q = q.Where("lower(to_email) = ?", strings.ToLower(gmail))
	}

	limit := emailLogDefaultLimit
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, emailLogMaxLimit)
		}
	}

	var rows []entity.EmailDelivery
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		log.Printf("admin: อ่าน email_deliveries ไม่สำเร็จ: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อ่านประวัติการส่งอีเมลไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, rows)
}
