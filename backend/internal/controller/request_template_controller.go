package controller

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/entity"
	"backend/internal/utils"
)

// RequestTemplateController ให้ผู้ใช้ดู "choices" ที่ admin เปิดไว้ (ฝั่งสร้าง/แก้ อยู่ที่ AdminController)
type RequestTemplateController struct{ db *gorm.DB }

// NewRequestTemplateController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewRequestTemplateController(db *gorm.DB) *RequestTemplateController {
	return &RequestTemplateController{db: db}
}

// List คืน template ที่เปิดใช้งานอยู่ ให้ผู้ใช้เลือกตอนจะยื่น request หรือ deploy service
// data flow: SELECT request_templates WHERE is_active = true → ตอบเป็น array
// → frontend เอาไปโชว์เป็นตัวเลือก แล้วส่ง request_template_id กลับมาตอน POST /api/requests หรือ /api/services
func (h *RequestTemplateController) List(c *gin.Context) {
	var templates []entity.RequestTemplate
	err := h.db.WithContext(c.Request.Context()).
		Where("is_active = true").Order("cpu_limit_milli").Find(&templates).Error
	if err != nil {
		log.Printf("list request templates error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, templates)
}
