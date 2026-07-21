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

// RequestController ดูแลคำขอสร้าง VM/namespace ของผู้ใช้ — ยื่นคำขอ, ดูประวัติของตัวเอง
// การอนุมัติ/ปฏิเสธเป็นหน้าที่ของ AdminController (ต้องผ่าน AdminOnly)
type RequestController struct {
	db *gorm.DB
}

// NewRequestController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewRequestController(db *gorm.DB) *RequestController {
	return &RequestController{db: db}
}

// Create ยื่นคำขอสร้าง VM/namespace ใหม่ (รอ admin อนุมัติ)
//
// data flow: JSON body → bind CreateRequestRequest → เช็คว่ายังไม่มี space และยังไม่มีคำขอค้างอยู่
// → INSERT requests (status = pending)
//
// เช็คซ้ำที่นี่เป็นแค่ด่านแรกกันคำขอกองซ้อน — ด่านสุดท้ายจริงๆ คือ NamespaceManager.Create
// ตอน admin approve (ErrAlreadyInNamespace) ซึ่งกันไว้อีกชั้นอยู่แล้ว
func (h *RequestController) Create(c *gin.Context) {
	var req dto.CreateRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	userID := c.GetInt("userID")
	ctx := c.Request.Context()

	var user entity.User
	if err := h.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		log.Printf("create request: find user error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	if user.NamespaceID != nil {
		utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", "คุณมี namespace อยู่แล้ว (1 คน = 1 space)")
		return
	}

	var pendingCount int64
	h.db.WithContext(ctx).Model(&entity.Request{}).
		Where("user_id = ? AND status = ?", userID, entity.RequestPending).Count(&pendingCount)
	if pendingCount > 0 {
		utils.Error(c, http.StatusConflict, "REQUEST_PENDING", "คุณมีคำขอที่รอการอนุมัติอยู่แล้ว")
		return
	}

	// ถ้าอ้างอิง template มา ก๊อป storage_gb ของมันมาเก็บเป็น snapshot ไว้กับคำขอ
	// (cpu/ram ยังเชื่อค่าที่ client ส่งมาเหมือนเดิม — WorkspaceOnboarding ก๊อปมาจาก template อยู่แล้ว)
	var storageGB int
	if req.RequestTemplateID != nil {
		var tmpl entity.RequestTemplate
		if err := h.db.WithContext(ctx).
			Where("id = ? AND is_active = true", *req.RequestTemplateID).First(&tmpl).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.Error(c, http.StatusBadRequest, "TEMPLATE_NOT_FOUND", "ไม่พบ template ที่เลือก (หรือถูกปิดใช้งานแล้ว)")
				return
			}
			log.Printf("create request: find template error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
		storageGB = tmpl.StorageGB
	}

	request := entity.Request{
		Description:       req.Description,
		UserID:            userID,
		Status:            entity.RequestPending,
		NamespaceName:     req.NamespaceName,
		RequestTemplateID: req.RequestTemplateID,
		CPULimitMilli:     req.CPULimitMilli,
		RAMLimitMB:        req.RAMLimitMB,
		StorageGB:         storageGB,
	}
	if err := h.db.WithContext(ctx).Create(&request).Error; err != nil {
		log.Printf("create request error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ส่งคำขอไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusCreated, request)
}

// ListMine คืนประวัติคำขอทั้งหมดของผู้ใช้ที่ล็อกอินอยู่ (ใหม่สุดขึ้นก่อน)
func (h *RequestController) ListMine(c *gin.Context) {
	userID := c.GetInt("userID")

	var requests []entity.Request
	err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).Order("created_at DESC").Find(&requests).Error
	if err != nil {
		log.Printf("list my requests error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, requests)
}
