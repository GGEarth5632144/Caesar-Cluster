package controller

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/dto"
	"backend/internal/services"
	"backend/internal/utils"
)

// InviteController ดูแลคำเชิญเข้ากลุ่ม: เจ้าของ space เชิญ/ดู/ยกเลิกคำเชิญที่ส่งไป,
// ผู้ถูกเชิญดู/ตอบรับ/ปฏิเสธคำเชิญที่ส่งถึงตัวเอง — เป็นชั้นบางๆ คุยกับ InviteManager เท่านั้น
type InviteController struct {
	inv *services.InviteManager
}

// NewInviteController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewInviteController(inv *services.InviteManager) *InviteController {
	return &InviteController{inv: inv}
}

// Create ให้เจ้าของ (contributor) ของ space ตัวเองเชิญ student_id คนหนึ่งเข้ากลุ่ม
func (h *InviteController) Create(c *gin.Context) {
	var req dto.CreateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	invite, err := h.inv.Create(c.Request.Context(), c.GetInt("userID"), req.StudentID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNoNamespace):
			utils.Error(c, http.StatusConflict, "NO_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNotContributor):
			utils.Error(c, http.StatusForbidden, "NOT_CONTRIBUTOR", err.Error())
		case errors.Is(err, services.ErrInviteSelf):
			utils.Error(c, http.StatusBadRequest, "INVITE_SELF", err.Error())
		case errors.Is(err, services.ErrStudentNotEligible):
			utils.Error(c, http.StatusBadRequest, "STUDENT_NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrStudentNotCPE):
			utils.Error(c, http.StatusBadRequest, "NOT_CPE", err.Error())
		case errors.Is(err, services.ErrStudentNotActive):
			utils.Error(c, http.StatusBadRequest, "NOT_ACTIVE_STUDENT", err.Error())
		case errors.Is(err, services.ErrInviteeAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrInviteAlreadyPending):
			utils.Error(c, http.StatusConflict, "INVITE_ALREADY_PENDING", err.Error())
		default:
			log.Printf("create invite error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ส่งคำเชิญไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusCreated, invite)
}

// Mine คืนคำเชิญที่ pending อยู่ ส่งถึงตัวเอง (ใช้โชว์เป็น "คำเชิญรอตอบรับ" บน dashboard)
func (h *InviteController) Mine(c *gin.Context) {
	list, err := h.inv.Mine(c.Request.Context(), c.GetInt("userID"))
	if err != nil {
		log.Printf("list my invites error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// Sent คืนคำเชิญทั้งหมด (ทุกสถานะ) ที่ตัวเอง (เจ้าของ) เคยส่งจาก space ของตัวเอง
func (h *InviteController) Sent(c *gin.Context) {
	list, err := h.inv.Sent(c.Request.Context(), c.GetInt("userID"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNoNamespace):
			utils.Error(c, http.StatusConflict, "NO_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNotContributor):
			utils.Error(c, http.StatusForbidden, "NOT_CONTRIBUTOR", err.Error())
		default:
			log.Printf("list sent invites error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		}
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// Accept ให้ผู้ถูกเชิญตอบรับคำเชิญ — เท่ากับ join namespace ที่เชิญมาทันที
func (h *InviteController) Accept(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	ns, err := h.inv.Accept(c.Request.Context(), c.GetInt("userID"), id)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInviteNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrInviteWrongUser):
			utils.Error(c, http.StatusForbidden, "FORBIDDEN", err.Error())
		case errors.Is(err, services.ErrInviteNotPending):
			utils.Error(c, http.StatusConflict, "INVITE_NOT_PENDING", err.Error())
		case errors.Is(err, services.ErrAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		default:
			log.Printf("accept invite error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ตอบรับคำเชิญไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, ns)
}

// Decline ให้ผู้ถูกเชิญปฏิเสธคำเชิญ
func (h *InviteController) Decline(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if err := h.inv.Decline(c.Request.Context(), c.GetInt("userID"), id); err != nil {
		switch {
		case errors.Is(err, services.ErrInviteNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrInviteWrongUser):
			utils.Error(c, http.StatusForbidden, "FORBIDDEN", err.Error())
		case errors.Is(err, services.ErrInviteNotPending):
			utils.Error(c, http.StatusConflict, "INVITE_NOT_PENDING", err.Error())
		default:
			log.Printf("decline invite error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ปฏิเสธคำเชิญไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"message": "ปฏิเสธคำเชิญแล้ว"})
}

// Cancel ให้เจ้าของ (contributor) ยกเลิกคำเชิญที่ตัวเองส่งไป (เผื่อเชิญผิดคน)
func (h *InviteController) Cancel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if err := h.inv.Cancel(c.Request.Context(), c.GetInt("userID"), id); err != nil {
		switch {
		case errors.Is(err, services.ErrNoNamespace):
			utils.Error(c, http.StatusConflict, "NO_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNotContributor):
			utils.Error(c, http.StatusForbidden, "NOT_CONTRIBUTOR", err.Error())
		case errors.Is(err, services.ErrInviteNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrInviteNotPending):
			utils.Error(c, http.StatusConflict, "INVITE_NOT_PENDING", err.Error())
		default:
			log.Printf("cancel invite error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ยกเลิกคำเชิญไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"message": "ยกเลิกคำเชิญแล้ว"})
}
