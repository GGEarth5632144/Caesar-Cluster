package controller

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/dto"
	"backend/internal/services"
	"backend/internal/utils"
)

// ServiceController เป็นชั้นบางๆ ระหว่าง HTTP กับ ServiceManager — แปลง request/response เท่านั้น
// logic จริง (เช็คโควตา, deploy ขึ้น cluster) อยู่ใน service layer ทั้งหมด
type ServiceController struct {
	db  *gorm.DB
	svc *services.ServiceManager
}

// NewServiceController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewServiceController(db *gorm.DB, svc *services.ServiceManager) *ServiceController {
	return &ServiceController{db: db, svc: svc}
}

// List คืน service ทั้งหมดใน space ของผู้ใช้
// data flow: currentNamespaceID → ServiceManager.ListByNamespace → ตอบเป็น array
// สมาชิกทุกคนในกลุ่มเห็น service ของกลุ่มเหมือนกันหมด (โควตาเป็นของ space ร่วมกัน)
func (h *ServiceController) List(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	list, err := h.svc.ListByNamespace(c.Request.Context(), nsID)
	if err != nil {
		log.Printf("list services error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// Create deploy service ใหม่เข้า space ของผู้ใช้
//
// data flow: JSON body → bind CreateServiceRequest → เช็คชื่อตามกฎ k8s
// → แปลงเป็น services.CreateServiceParams → ServiceManager.Create
// (เลือก template หรือกรอกสเปกเอง → เช็คโควตารวมของ namespace → INSERT → deploy จริง) → ตอบ service ที่สร้าง
//
// error ที่ผู้ใช้แก้เองได้จะถูกแปลงเป็น 409 พร้อมบอกเหตุผล (โควตาไม่พอ / service เต็ม / สเปกเกินเพดาน)
func (h *ServiceController) Create(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	var req dto.CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if !isValidK8sName(req.Name) {
		utils.Error(c, http.StatusBadRequest, "INVALID_NAME",
			"ชื่อต้องเป็นตัวพิมพ์เล็ก/ตัวเลข/ขีดกลาง และขึ้นต้น-ลงท้ายด้วยตัวอักษรหรือตัวเลข")
		return
	}

	svc, err := h.svc.Create(c.Request.Context(), c.GetInt("userID"), nsID, services.CreateServiceParams{
		Name:              req.Name,
		Image:             req.Image,
		RequestTemplateID: req.RequestTemplateID,
		CPUMilli:          req.CPUMilli,
		RAMMB:             req.RAMMB,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrQuotaExceeded):
			utils.Error(c, http.StatusConflict, "QUOTA_EXCEEDED", err.Error())
		case errors.Is(err, services.ErrServiceLimit):
			utils.Error(c, http.StatusConflict, "SERVICE_LIMIT", err.Error())
		case errors.Is(err, services.ErrServiceTooLarge):
			utils.Error(c, http.StatusBadRequest, "SERVICE_TOO_LARGE", err.Error())
		case errors.Is(err, services.ErrRequestTemplateNotFound):
			utils.Error(c, http.StatusBadRequest, "TEMPLATE_NOT_FOUND", err.Error())
		default:
			log.Printf("create service error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "deploy ไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusCreated, svc)
}

// Delete ลบ service ออกจาก space ของผู้ใช้ (คืนโควตาให้ namespace ทันที)
//
// data flow: อ่าน id จาก path + namespace ของผู้ใช้ → ServiceManager.Delete
// (ถอน workload จริงบน cluster ก่อน แล้วค่อยลบ row) → ตอบ deleted:id
//
// ลบได้เฉพาะ service ที่อยู่ใน namespace ของตัวเอง — ของ space อื่นจะเจอ 404
func (h *ServiceController) Delete(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id, nsID); err != nil {
		if errors.Is(err, services.ErrServiceNotFound) {
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		log.Printf("delete service error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, gin.H{"deleted": id})
}
