package controller

import (
	"net/http"

	"backend/internal/services" 
	"github.com/gin-gonic/gin"
)

type TelemetryController struct {
	telemetrySvc *services.TelemetryService
}

func NewTelemetryController(svc *services.TelemetryService) *TelemetryController {
	return &TelemetryController{
		telemetrySvc: svc,
	}
}

func (tc *TelemetryController) GetTelemetry(c *gin.Context) {
	// ไปดึงข้อมูลจาก Database ผ่าน Service
	data, err := tc.telemetrySvc.GetAllTelemetry()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch telemetry data"})
		return
	}

	// ส่งข้อมูลกลับไปเป็น JSON ให้ React
	c.JSON(http.StatusOK, data)
}