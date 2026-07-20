package controller

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/dto"
	"backend/internal/entity"
	"backend/internal/services"
	"backend/internal/utils"
)

// AdminController รวม endpoint ฝั่ง admin ไว้ที่เดียว:
// import รายชื่อ นศ. ที่มีสิทธิ์, สร้าง choices (plans), ดูภาพรวม namespace, ปรับโควตาให้กลุ่ม
// ทุก route ที่ผูกกับ controller นี้ผ่าน middleware AdminOnly มาแล้ว
type AdminController struct {
	db *gorm.DB
	ns *services.NamespaceManager
}

// NewAdminController ประกอบ controller — ถูกเรียกจาก router.Setup
func NewAdminController(db *gorm.DB, ns *services.NamespaceManager) *AdminController {
	return &AdminController{db: db, ns: ns}
}

// AddEligibleStudents import รายชื่อ นศ. ที่มีสิทธิ์สมัครใช้งาน (ตาราง "match" ใน ERD)
//
// data flow: JSON body (array) → bind AddEligibleStudentRequest
// → INSERT eligible_students แบบ ON CONFLICT DO NOTHING (import ทับซ้ำได้ ไม่พัง)
// → ตอบจำนวนที่เพิ่มเข้าไปจริง
//
// นี่คือประตูเดียวที่ทำให้ใครสมัครได้ — ถ้า student_id ไม่อยู่ในตารางนี้ Register จะตอบ 403 เสมอ
func (h *AdminController) AddEligibleStudents(c *gin.Context) {
	var req dto.AddEligibleStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	rows := make([]entity.EligibleStudent, 0, len(req.Students))
	for _, s := range req.Students {
		rows = append(rows, entity.EligibleStudent{StudentID: s.StudentID, Major: s.Major})
	}

	// ซ้ำแล้วข้ามไป (idempotent) — admin import ไฟล์เดิมซ้ำได้โดยไม่ error
	res := h.db.WithContext(c.Request.Context()).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&rows)
	if res.Error != nil {
		log.Printf("add eligible students error: %v", res.Error)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เพิ่มรายชื่อไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusCreated, gin.H{
		"submitted": len(rows),
		"inserted":  res.RowsAffected, // ที่เหลือคือรายชื่อที่มีอยู่แล้ว
	})
}

// CreateRequestTemplate สร้าง "choice" ใหม่ให้ผู้ใช้เลือก (เช่น small = 500m/512MB)
// data flow: JSON body → bind CreateRequestTemplateRequest → INSERT request_templates (is_active = true)
// → ตอบ template ที่สร้าง → ผู้ใช้จะเห็นทันทีที่ GET /api/request-templates
func (h *AdminController) CreateRequestTemplate(c *gin.Context) {
    var req dto.CreateRequestTemplateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
        return
    }

    tmpl := entity.RequestTemplate{
        OptionName:      req.OptionName, 
        Category:        req.Category,
        Description:     req.Description,
        RelateSubject:   req.RelateSubject,
        CPULimitMilli:   req.CPULimitMilli,
        RAMLimitMB:      req.RAMLimitMB,
        StorageGB:       req.StorageGB, 
        IsActive:        false,
    }
    
    if err := h.db.WithContext(c.Request.Context()).Create(&tmpl).Error; err != nil {
        utils.Error(c, http.StatusConflict, "TEMPLATE_EXISTS", "ชื่อ template นี้มีอยู่แล้วหรือข้อมูลไม่ถูกต้อง")
        return
    }
    utils.OK(c, http.StatusCreated, tmpl)
}

// UpdateRequestTemplate แก้ไขข้อมูล Template หรือเปิด/ปิดสถานะ (PATCH)
// data flow: อ่าน id จาก path + JSON body → ค้นหาใน DB → อัปเดตข้อมูล → ตอบข้อมูลที่อัปเดตแล้ว
func (h *AdminController) UpdateRequestTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var req dto.UpdateRequestTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	// ค้นหา Template เดิมก่อน
	var tmpl entity.RequestTemplate
	if err := h.db.WithContext(c.Request.Context()).First(&tmpl, id).Error; err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบ template นี้ในระบบ")
		return
	}

	// อัปเดตเฉพาะฟิลด์ที่มีการส่งค่ามา (ใช้ Map เพื่อให้รองรับการอัปเดตแบบ Partial หรือบางฟิลด์)
	updates := make(map[string]interface{})
	
	if req.OptionName != nil { updates["name"] = *req.OptionName }
	if req.Category != nil { updates["category"] = *req.Category }
	if req.Description != nil { updates["description"] = *req.Description }
	if req.RelateSubject != nil { updates["relate_subject"] = *req.RelateSubject }
	if req.CPULimitMilli != nil { updates["cpu_limit_milli"] = *req.CPULimitMilli }
	if req.RAMLimitMB != nil { updates["ram_limit_mb"] = *req.RAMLimitMB }
	if req.StorageGB != nil { updates["storage_gb"] = *req.StorageGB }
	if req.IsActive != nil { updates["is_active"] = *req.IsActive } // สำคัญมาก สำหรับ Checkbox เปิด/ปิด

	if err := h.db.WithContext(c.Request.Context()).Model(&tmpl).Updates(updates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "อัปเดตข้อมูลไม่สำเร็จ")
		return
	}

	// ดึงข้อมูลล่าสุดกลับมาตอบกลับ
	h.db.First(&tmpl, id)
	utils.OK(c, http.StatusOK, tmpl)
}

func (h *AdminController) ListAllRequestTemplates(c *gin.Context) {
	var templates []entity.RequestTemplate
	// ไม่ต้องใส่ Where("is_active = true") เพื่อดึงมาทั้งหมด
	if err := h.db.WithContext(c.Request.Context()).Order("id").Find(&templates).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ดึงข้อมูลไม่สำเร็จ")
		return
	}
	utils.OK(c, http.StatusOK, templates)
}

// DeleteRequestTemplate ลบ Template ออกจากระบบ (DELETE)
func (h *AdminController) DeleteRequestTemplate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	// ลบข้อมูลจากฐานข้อมูล
	if err := h.db.WithContext(c.Request.Context()).Delete(&entity.RequestTemplate{}, id).Error; err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ลบข้อมูลไม่สำเร็จ")
		return
	}

	utils.OK(c, http.StatusOK, gin.H{"message": "ลบเทมเพลตสำเร็จ"})
}

// ListNamespaces คืน namespace ทั้งหมดในระบบ พร้อมยอดใช้งานและจำนวนสมาชิก (หน้าภาพรวมของ admin)
// data flow: NamespaceManager.ListAll (SELECT namespaces + SUM ทรัพยากร + COUNT สมาชิกของแต่ละอัน) → ตอบ array
func (h *AdminController) ListNamespaces(c *gin.Context) {
	list, err := h.ns.ListAll(c.Request.Context())
	if err != nil {
		log.Printf("list namespaces error: %v", err)
		utils.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
		return
	}
	utils.OK(c, http.StatusOK, list)
}

// SetNamespaceQuota ปรับโควตาของ namespace (เช่น อัปกลุ่มจาก 3 core เป็น 8 core)
//
// data flow: อ่าน id จาก path + JSON body → bind SetQuotaRequest → NamespaceManager.SetQuota
// (ตรวจเพดานตามชนิด space → UPDATE namespaces → sync ResourceQuota ขึ้น cluster) → ตอบ namespace ที่อัปเดตแล้ว
//
// เพดาน: กลุ่มไม่เกิน 8 core / 8 GB, เดี่ยวไม่เกินค่า default (3 core / 2 GB)
func (h *AdminController) SetNamespaceQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_ID", "id ต้องเป็นตัวเลข")
		return
	}

	var req dto.SetQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	detail, err := h.ns.SetQuota(c.Request.Context(), id, req.CPULimitMilli, req.RAMLimitMB, req.MaxServices)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrNamespaceNotFound):
			utils.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, services.ErrQuotaOutOfRange):
			utils.Error(c, http.StatusBadRequest, "QUOTA_OUT_OF_RANGE", err.Error())
		default:
			log.Printf("set quota error: %v", err)
			utils.Error(c, http.StatusInternalServerError, "INTERNAL", "ปรับโควตาไม่สำเร็จ")
		}
		return
	}
	utils.OK(c, http.StatusOK, detail)
}
