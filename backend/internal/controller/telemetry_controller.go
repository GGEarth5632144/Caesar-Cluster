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

func (tc *TelemetryController) GetTelemetryHistory(c *gin.Context) {
    // รับค่า range จาก Frontend เช่น 1h, 6h, 24h
    timeRange := c.DefaultQuery("range", "1h")

    // ให้ Service ไปยิง Prometheus API แล้วแปลงข้อมูลกลับมาเป็น Array 
    // Format: [{time: "10:00", avgTemp: 45, totalRam: 16000, onlineNodes: 38}, ...]
    historyData, err := tc.telemetrySvc.GetHistoryFromPrometheus(timeRange)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prometheus data"})
        return
    }

    c.JSON(http.StatusOK, historyData)
}