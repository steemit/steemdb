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

// WitnessHandler handles witness-related HTTP requests
type WitnessHandler struct {
	witnessService *services.WitnessService
	logger         utils.Logger
}

// NewWitnessHandler creates a new witness handler
func NewWitnessHandler(witnessService *services.WitnessService, logger utils.Logger) *WitnessHandler {
	return &WitnessHandler{
		witnessService: witnessService,
		logger:         logger,
	}
}

// GetWitnesses handles GET /api/v1/witnesses
func (h *WitnessHandler) GetWitnesses(c *gin.Context) {
	page := 1
	limit := 50

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	sortBy := c.DefaultQuery("sort", "votes")
	sortOrder := c.DefaultQuery("order", "desc")

	result, err := h.witnessService.GetWitnesses(c.Request.Context(), page, limit, sortBy, sortOrder)
	if err != nil {
		h.logger.Error("Failed to get witnesses", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve witnesses")
		return
	}

	h.respondWithSuccessAndMeta(c, result.Witnesses, &models.Meta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: result.TotalPages,
	})
}

// GetTopWitnesses handles GET /api/v1/witnesses/top
func (h *WitnessHandler) GetTopWitnesses(c *gin.Context) {
	limit := 21

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	witnesses, err := h.witnessService.GetTopWitnesses(c.Request.Context(), limit)
	if err != nil {
		h.logger.Error("Failed to get top witnesses", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve top witnesses")
		return
	}

	h.respondWithSuccess(c, witnesses)
}

// GetWitness handles GET /api/v1/witnesses/:username
func (h *WitnessHandler) GetWitness(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		h.respondWithError(c, http.StatusBadRequest, "Witness username is required")
		return
	}

	witness, err := h.witnessService.GetWitness(c.Request.Context(), username)
	if err != nil {
		h.logger.Error("Failed to get witness", utils.String("username", username), utils.Error(err))
		h.respondWithError(c, http.StatusNotFound, "Witness not found")
		return
	}

	h.respondWithSuccess(c, witness)
}

// Helper methods

func (h *WitnessHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *WitnessHandler) respondWithSuccessAndMeta(c *gin.Context, data interface{}, meta *models.Meta) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	})
}

func (h *WitnessHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
