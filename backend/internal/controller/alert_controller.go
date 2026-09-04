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

// AlertController = ชั้นบางๆ ระหว่าง HTTP กับ AlertManager
//
// ทุก endpoint ทำงานกับ "แจ้งเตือนของผู้เรียกเท่านั้น" — userID มาจาก JWT ที่ middlewares.Auth
// ใส่ไว้ใน context ไม่ได้มาจาก body/query จึงไม่มีทางอ่านหรือแก้ของคนอื่นด้วยการเดา id
type AlertController struct {
	alerts *services.AlertManager
}

// NewAlertController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewAlertController(alerts *services.AlertManager) *AlertController {
	return &AlertController{alerts: alerts}
}

// List คืนแจ้งเตือนของผู้ใช้ เรียงใหม่→เก่า
//
// query param: unread=true (เอาเฉพาะที่ยังไม่อ่าน), limit=<n>
// data flow: userID จาก JWT → AlertManager.ListForUser → array ของ UserAlert
func (h *AlertController) List(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	list, err := h.alerts.ListForUser(c.Request.Context(), c.GetInt("userID"), limit, c.Query("unread") == "true")
	if err != nil {
		log.Printf("list alerts error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงแจ้งเตือนไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// UnreadCount คืนจำนวนแจ้งเตือนที่ยังไม่ได้อ่าน — เป็นตัวเลขในวงกลมแดงบน Sidebar โดยตรง
//
// แยกเป็น endpoint ของตัวเองแทนที่จะให้ frontend ดึงทั้งรายการมานับเอง เพราะหน้าเว็บ poll
// ค่านี้ทุกนาทีจากทุกหน้า — ส่งแค่ตัวเลขตัวเดียวถูกกว่าส่งทั้งก้อนมาก
func (h *AlertController) UnreadCount(c *gin.Context) {
	n, err := h.alerts.UnreadCount(c.Request.Context(), c.GetInt("userID"))
	if err != nil {
		log.Printf("count unread alerts error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "นับแจ้งเตือนไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"unread": n})
}

// MarkRead ทำเครื่องหมายว่าอ่านแล้ว
//
// body: {"ids": [1,2,3]} เจาะจงเป็นรายตัว หรือ {"all": true} ทั้งหมดของผู้ใช้คนนี้
// ตอบจำนวนแถวที่เปลี่ยนสถานะจริง ให้หน้าเว็บเอาไปหักออกจากตัวเลขบน Sidebar ได้ทันทีโดยไม่ต้องยิงถามใหม่
func (h *AlertController) MarkRead(c *gin.Context) {
	var req dto.MarkAlertsReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if !req.All && len(req.IDs) == 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", "ต้องส่ง ids อย่างน้อยหนึ่งตัว หรือ all=true")
		return
	}

	ids := req.IDs
	if req.All {
		ids = nil // nil = ทั้งหมดของ user คนนี้ (ดู AlertManager.MarkRead)
	}
	n, err := h.alerts.MarkRead(c.Request.Context(), c.GetInt("userID"), ids)
	if err != nil {
		log.Printf("mark alerts read error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อัปเดตไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"updated": n})
}

// Delete ลบแจ้งเตือนทีละอัน — ลบได้เฉพาะของตัวเอง ของคนอื่นจะเจอ 404
func (h *AlertController) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if err := h.alerts.Delete(c.Request.Context(), c.GetInt("userID"), id); err != nil {
		if errors.Is(err, services.ErrAlertNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		log.Printf("delete alert error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"deleted": id})
}

// DeleteRead ล้างแจ้งเตือนที่อ่านแล้วทิ้งทั้งหมด (ไม่แตะตัวที่ยังไม่อ่าน)
func (h *AlertController) DeleteRead(c *gin.Context) {
	n, err := h.alerts.DeleteRead(c.Request.Context(), c.GetInt("userID"))
	if err != nil {
		log.Printf("clear read alerts error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ล้างไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"deleted": n})
}
