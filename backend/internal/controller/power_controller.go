package controller

import (
	"net/http"

	"backend/internal/services"
	"github.com/gin-gonic/gin"
)

type PowerController struct {
	PowerService *services.PowerService
}

func NewPowerController(powerService *services.PowerService) *PowerController {
	return &PowerController{PowerService: powerService}
}

func (c *PowerController) GetPower(ctx *gin.Context) {
	data, err := c.PowerService.GetAllPower()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// ส่งกลับเป็น Array ตรงๆ เหมือน telemetry
	ctx.JSON(http.StatusOK, data)
}

func (ctrl *PowerController) GetPowerHistory(c *gin.Context) {
	timeRange := c.DefaultQuery("timeRange", "1h")

	historyDto, err := ctrl.PowerService.GetPowerHistory(timeRange)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch power history"})
		return
	}

	c.JSON(http.StatusOK, historyDto)
}