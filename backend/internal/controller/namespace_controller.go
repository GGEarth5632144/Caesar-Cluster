package controller

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/dto"
	"backend/internal/services"
	"backend/internal/utils"
)

// NamespaceController ดูแล space ของผู้ใช้: สร้าง (เดี่ยว/กลุ่ม), เข้าร่วมกลุ่ม, ดู space ของตัวเอง
type NamespaceController struct {
	db *gorm.DB
	ns *services.NamespaceManager
}

// NewNamespaceController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewNamespaceController(db *gorm.DB, ns *services.NamespaceManager) *NamespaceController {
	return &NamespaceController{db: db, ns: ns}
}

// Create สร้าง space ให้ผู้ใช้ที่ล็อกอินอยู่ (เป็นได้ทั้งแบบเดี่ยวและกลุ่ม)
//
// data flow: JSON body → bind CreateNamespaceRequest → เช็คชื่อว่าถูกกฎ k8s
// → NamespaceManager.Create (INSERT namespaces + ผูก users.namespace_id + สร้างจริงบน cluster)
// → ตอบ namespace ที่สร้าง
//
// ผู้ใช้ที่มี space อยู่แล้วสร้างซ้ำไม่ได้ (1 คน = 1 space) → 409
func (h *NamespaceController) Create(c *gin.Context) {
	var req dto.CreateNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if !isValidK8sName(req.Name) {
		utils.Error(c, http.StatusBadRequest, "INVALID_NAME",
			"ชื่อต้องเป็นตัวพิมพ์เล็ก/ตัวเลข/ขีดกลาง และขึ้นต้น-ลงท้ายด้วยตัวอักษรหรือตัวเลข")
		return
	}

	ns, err := h.ns.Create(c.Request.Context(), c.GetInt("userID"), req.Name, req.Type)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNameTaken):
			utils.Error(c, http.StatusConflict, "NAME_TAKEN", err.Error())
		default:
			log.Printf("create namespace error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง namespace ไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusCreated, ns)
}

// Join พาผู้ใช้เข้าร่วม space แบบกลุ่มที่มีอยู่แล้ว
//
// data flow: JSON body → bind JoinNamespaceRequest → NamespaceManager.Join (UPDATE users.namespace_id)
// → ตอบ namespace ที่เข้าร่วม
//
// เข้าได้เฉพาะ space ชนิด group เท่านั้น และต้องยังไม่มี space ของตัวเอง
func (h *NamespaceController) Join(c *gin.Context) {
	var req dto.JoinNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	ns, err := h.ns.Join(c.Request.Context(), c.GetInt("userID"), req.NamespaceID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAlreadyInNamespace):
			utils.Error(c, http.StatusConflict, "ALREADY_IN_NAMESPACE", err.Error())
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrNotGroupNamespace):
			utils.Error(c, http.StatusConflict, "NOT_GROUP", err.Error())
		default:
			log.Printf("join namespace error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เข้าร่วมไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, ns)
}

// Mine คืน space ของผู้ใช้ พร้อมยอดใช้งานจริงและจำนวนสมาชิก (หน้า dashboard ใช้ตัวนี้)
//
// data flow: currentNamespaceID (อ่านจาก users.namespace_id) → NamespaceManager.Detail
// (namespace + SUM ทรัพยากรที่ใช้ + COUNT สมาชิก) → ตอบกลับ
// ถ้ายังไม่มี space → currentNamespaceID ตอบ 409 NO_NAMESPACE ให้แล้ว
func (h *NamespaceController) Mine(c *gin.Context) {
	nsID, ok := currentNamespaceID(c, h.db)
	if !ok {
		return
	}

	detail, err := h.ns.Detail(c.Request.Context(), nsID)
	if err != nil {
		log.Printf("namespace detail error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, detail)
}
