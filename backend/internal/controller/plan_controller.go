package controller

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/entity"
	"backend/internal/utils"
)

// PlanController ให้ผู้ใช้ดู "choices" ที่ admin เปิดไว้ (ฝั่งสร้าง/แก้ อยู่ที่ AdminController)
type PlanController struct{ db *gorm.DB }

// NewPlanController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewPlanController(db *gorm.DB) *PlanController { return &PlanController{db: db} }

// List คืน plan ที่เปิดใช้งานอยู่ ให้ผู้ใช้เลือกตอนจะ deploy service
// data flow: SELECT plans WHERE is_active = true → ตอบเป็น array
// → frontend เอาไปโชว์เป็นตัวเลือก แล้วส่ง plan_id กลับมาตอน POST /api/services
func (h *PlanController) List(c *gin.Context) {
	var plans []entity.Plan
	err := h.db.WithContext(c.Request.Context()).
		Where("is_active = true").Order("cpu_milli").Find(&plans).Error
	if err != nil {
		log.Printf("list plans error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, plans)
}
