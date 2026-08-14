package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// SearchHandler handles search-related HTTP requests
type SearchHandler struct {
	searchService *services.SearchService
	logger        utils.Logger
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(searchService *services.SearchService, logger utils.Logger) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
		logger:        logger,
	}
}

// Search handles GET /api/v1/search
func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		h.respondWithError(c, http.StatusBadRequest, "Search query is required")
		return
	}

	searchType := c.Query("type")

	results, err := h.searchService.Search(c.Request.Context(), query, searchType)
	if err != nil {
		h.logger.Error("Failed to search", utils.String("query", query), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Search failed")
		return
	}

	h.respondWithSuccess(c, results)
}

// Helper methods

func (h *SearchHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *SearchHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
