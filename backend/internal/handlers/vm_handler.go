package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/models"
	"backend/internal/response"
	"backend/internal/services"
)

type VMHandler struct{ vms *services.VMService }

func NewVMHandler(vms *services.VMService) *VMHandler { return &VMHandler{vms: vms} }

func (h *VMHandler) List(c *gin.Context) {
	list, err := h.vms.ListByOwner(c.Request.Context(), c.GetInt("userID"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	response.OK(c, http.StatusOK, list)
}

func (h *VMHandler) Create(c *gin.Context) {
	var req models.CreateVMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	vm, err := h.vms.Create(c.Request.Context(), c.GetInt("userID"), req)
	if err != nil {
		if errors.Is(err, services.ErrNoCapacity) {
			response.Error(c, http.StatusConflict, "NO_CAPACITY", err.Error())
			return
		}
		log.Printf("create vm error: %v", err)
		response.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง VM ไม่สำเร็จ")
		return
	}
	response.OK(c, http.StatusCreated, vm)
}

func (h *VMHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	deleted, err := h.vms.Delete(c.Request.Context(), id, c.GetInt("userID"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	if !deleted {
		response.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบ VM หรือไม่ใช่ของคุณ")
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": id})
}
