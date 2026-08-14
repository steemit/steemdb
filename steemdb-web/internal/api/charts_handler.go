package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// ChartsHandler handles chart data HTTP requests
type ChartsHandler struct {
	chartsService *services.ChartsService
	logger        utils.Logger
}

// NewChartsHandler creates a new charts handler
func NewChartsHandler(chartsService *services.ChartsService, logger utils.Logger) *ChartsHandler {
	return &ChartsHandler{
		chartsService: chartsService,
		logger:        logger,
	}
}

// GetAccountGrowth handles GET /api/v1/charts/accounts/growth
func (h *ChartsHandler) GetAccountGrowth(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	data, err := h.chartsService.GetAccountGrowth(c.Request.Context(), days)
	if err != nil {
		h.logger.Error("Failed to get account growth chart", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve account growth data")
		return
	}

	h.respondWithSuccess(c, data)
}

// GetBlockProduction handles GET /api/v1/charts/blocks/production
func (h *ChartsHandler) GetBlockProduction(c *gin.Context) {
	days := 7
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	data, err := h.chartsService.GetBlockProduction(c.Request.Context(), days)
	if err != nil {
		h.logger.Error("Failed to get block production chart", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve block production data")
		return
	}

	h.respondWithSuccess(c, data)
}

// GetTransactionVolume handles GET /api/v1/charts/transactions/volume
func (h *ChartsHandler) GetTransactionVolume(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	data, err := h.chartsService.GetTransactionVolume(c.Request.Context(), days)
	if err != nil {
		h.logger.Error("Failed to get transaction volume chart", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve transaction volume data")
		return
	}

	h.respondWithSuccess(c, data)
}

// GetWitnessVoting handles GET /api/v1/charts/witnesses/voting
func (h *ChartsHandler) GetWitnessVoting(c *gin.Context) {
	data, err := h.chartsService.GetWitnessVoting(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get witness voting chart", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve witness voting data")
		return
	}

	h.respondWithSuccess(c, data)
}

// Helper methods

func (h *ChartsHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *ChartsHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
