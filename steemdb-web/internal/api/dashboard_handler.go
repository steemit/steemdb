package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// DashboardHandler handles dashboard-related HTTP requests
type DashboardHandler struct {
	dashboardService *services.DashboardService
	logger           utils.Logger
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(dashboardService *services.DashboardService, logger utils.Logger) *DashboardHandler {
	return &DashboardHandler{
		dashboardService: dashboardService,
		logger:           logger,
	}
}

// GetDashboard handles GET /api/v1/dashboard
func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	data, err := h.dashboardService.GetDashboardData(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get dashboard data", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve dashboard data")
		return
	}

	h.respondWithSuccess(c, data)
}

// Helper methods

func (h *DashboardHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      data,
		"timestamp": time.Now(),
	})
}

func (h *DashboardHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"success": false,
		"error": gin.H{
			"code":    statusCode,
			"message": message,
		},
		"timestamp": time.Now(),
	})
}
