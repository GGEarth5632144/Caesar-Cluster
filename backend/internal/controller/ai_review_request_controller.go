package controller

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/utils"
)

// AIReviewRequestController เก็บ/คืน "ใบเสร็จ" ของ deploy request ที่ส่งเข้า Cluster-AI เพื่อรอ review
//
// เหตุผลที่แยก resource นี้ออกจาก ServiceController: Cluster-AI (คนละ repo, คนละระบบ) เก็บแค่สถานะ
// pipeline ของตัวเองไว้ใน memory ล้วนๆ ไม่เก็บ service_name/image/cpu/ram ที่ Caesar-Cluster ส่งไปตอน submit
// (ดู Cluster-AI/Backend/store/review_store.go) — ถ้า AIReviewPage.tsx ฝั่ง frontend โดน refresh
// router state (location.state) ที่เคยพกข้อมูลพวกนี้มาจะหายหมด แล้วกด deploy จริงตอนท้ายไม่ได้เลย
// controller นี้เลยมีไว้เป็นที่เดียวที่ยังหาข้อมูลตั้งต้นของ request กลับมาได้ — ไม่เกี่ยวกับสถานะ pipeline
// (queued/reviewing/...) ซึ่งยังคง poll ตรงไปที่ Cluster-AI เหมือนเดิมทุกอย่าง ไม่มีอะไรเปลี่ยน
//
// เหมือน RequestTemplateController: logic ง่ายพอที่จะคุย db ตรงๆ ในนี้ ไม่ต้องมี service-layer manager แยก
// (ไม่มีเรื่องโควตา/provisioner เข้ามาเกี่ยวข้องเหมือน ServiceController)
type AIReviewRequestController struct{ db *gorm.DB }

// NewAIReviewRequestController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewAIReviewRequestController(db *gorm.DB) *AIReviewRequestController {
	return &AIReviewRequestController{db: db}
}

// Create บันทึก "ใบเสร็จ" ของ request ที่กำลังจะยิงไป Cluster-AI
// data flow: RequestQuotar.tsx (handleDeployWithAI) ยิงมาที่นี่คู่ขนานกับตอนยิง POST /api/review
// ไปที่ Cluster-AI โดยตรง (คนละ call กัน ไม่ block กัน — Caesar-Cluster ไม่ได้ proxy การเรียก Cluster-AI)
func (h *AIReviewRequestController) Create(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	var req dto.CreateAIReviewRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if !isValidK8sName(req.ServiceName) {
		utils.Error(c, http.StatusBadRequest, "INVALID_NAME",
			"ชื่อต้องเป็นตัวพิมพ์เล็ก/ตัวเลข/ขีดกลาง และขึ้นต้น-ลงท้ายด้วยตัวอักษรหรือตัวเลข")
		return
	}
	if !isValidEnvVars(req.EnvVars) {
		utils.Error(c, http.StatusBadRequest, "INVALID_ENV_VARS",
			"env vars ต้องมีไม่เกิน 20 ตัว ชื่อ key เป็นตัวอักษร/เลข/underscore และขึ้นต้นด้วยตัวอักษรหรือ underscore เท่านั้น")
		return
	}

	rec := entity.AIReviewRequest{
		RequestID:         req.RequestID,
		NamespaceID:       nsID,
		CreatedBy:         c.GetInt("userID"),
		ServiceName:       req.ServiceName,
		Image:             req.Image,
		RequestTemplateID: req.RequestTemplateID,
		CPUMilli:          req.CPUMilli,
		RAMMB:             req.RAMMB,
		EnvVars:           entity.EnvVarMap(req.EnvVars),
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&rec).Error; err != nil {
		log.Printf("create ai review request error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "บันทึกคำขอไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusCreated, rec)
}

// Get คืน "ใบเสร็จ" ตาม request_id — เฉพาะของ namespace ตัวเองเท่านั้น (เหมือน ServiceController.Delete)
// data flow: AIReviewPage.tsx เรียกตอน location.state หาย (refresh/เปิดลิงก์ตรง) → ใช้ข้อมูลนี้แทน
func (h *AIReviewRequestController) Get(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	requestID := c.Param("request_id")
	var rec entity.AIReviewRequest
	err := h.db.WithContext(c.Request.Context()).
		Where("request_id = ? AND namespace_id = ?", requestID, nsID).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบคำขอนี้ — อาจถูกส่งจาก namespace อื่น หรือ request_id ไม่ถูกต้อง")
			return
		}
		log.Printf("get ai review request error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, rec)
}
