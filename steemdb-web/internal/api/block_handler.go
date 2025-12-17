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

// BlockHandler handles block-related HTTP requests
type BlockHandler struct {
	blockService *services.BlockService
	logger       utils.Logger
}

// NewBlockHandler creates a new block handler
func NewBlockHandler(blockService *services.BlockService, logger utils.Logger) *BlockHandler {
	return &BlockHandler{
		blockService: blockService,
		logger:       logger,
	}
}

// GetBlock handles GET /api/v1/blocks/:number
func (h *BlockHandler) GetBlock(c *gin.Context) {
	numberStr := c.Param("number")
	if numberStr == "" {
		h.respondWithError(c, http.StatusBadRequest, "Block number is required")
		return
	}

	blockNum, err := strconv.ParseInt(numberStr, 10, 64)
	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid block number")
		return
	}

	block, err := h.blockService.GetBlock(c.Request.Context(), blockNum)
	if err != nil {
		h.logger.Error("Failed to get block", utils.Int64("number", blockNum), utils.Error(err))
		h.respondWithError(c, http.StatusNotFound, "Block not found")
		return
	}

	h.respondWithSuccess(c, block)
}

// GetBlocks handles GET /api/v1/blocks
func (h *BlockHandler) GetBlocks(c *gin.Context) {
	// Parse pagination parameters
	params := models.PaginationParams{
		Page:     1,
		PageSize: 20,
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			params.Page = p
		}
	}

	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
			params.PageSize = ps
		}
	}

	// Parse sort parameters
	sortParams := models.SortParams{
		SortBy:    c.DefaultQuery("sort_by", "number"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	result, err := h.blockService.GetBlocks(c.Request.Context(), params, sortParams)
	if err != nil {
		h.logger.Error("Failed to get blocks", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve blocks")
		return
	}

	h.respondWithSuccessAndMeta(c, result.Blocks, &models.Meta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize)),
	})
}

// GetLatestBlocks handles GET /api/v1/blocks/latest
func (h *BlockHandler) GetLatestBlocks(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	blocks, err := h.blockService.GetLatestBlocks(c.Request.Context(), limit)
	if err != nil {
		h.logger.Error("Failed to get latest blocks", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve latest blocks")
		return
	}

	h.respondWithSuccess(c, blocks)
}

// GetBlockStats handles GET /api/v1/blocks/stats
func (h *BlockHandler) GetBlockStats(c *gin.Context) {
	stats, err := h.blockService.GetBlockStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get block stats", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve block statistics")
		return
	}

	h.respondWithSuccess(c, stats)
}

// GetOperationStats handles GET /api/v1/operations/stats
func (h *BlockHandler) GetOperationStats(c *gin.Context) {
	timeRange := c.DefaultQuery("range", "24h")

	stats, err := h.blockService.GetOperationStats(c.Request.Context(), timeRange)
	if err != nil {
		h.logger.Error("Failed to get operation stats", utils.String("range", timeRange), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve operation statistics")
		return
	}

	h.respondWithSuccess(c, stats)
}

// Helper methods

func (h *BlockHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *BlockHandler) respondWithSuccessAndMeta(c *gin.Context, data interface{}, meta *models.Meta) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	})
}

func (h *BlockHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
