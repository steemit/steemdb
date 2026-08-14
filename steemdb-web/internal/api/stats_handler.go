package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// StatsHandler handles statistics-related HTTP requests
type StatsHandler struct {
	statsService *services.StatsService
	logger       utils.Logger
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(statsService *services.StatsService, logger utils.Logger) *StatsHandler {
	return &StatsHandler{
		statsService: statsService,
		logger:       logger,
	}
}

// GetGlobalStats handles GET /api/v1/stats/global
func (h *StatsHandler) GetGlobalStats(c *gin.Context) {
	stats, err := h.statsService.GetGlobalStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get global stats", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve global statistics")
		return
	}

	h.respondWithSuccess(c, stats)
}

// GetBlockchainProps handles GET /api/v1/stats/props
func (h *StatsHandler) GetBlockchainProps(c *gin.Context) {
	props, err := h.statsService.GetBlockchainProps(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get blockchain props", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve blockchain properties")
		return
	}

	h.respondWithSuccess(c, props)
}

// Helper methods

func (h *StatsHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *StatsHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
