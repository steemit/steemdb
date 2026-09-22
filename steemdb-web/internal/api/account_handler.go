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

// AccountHandler handles account-related HTTP requests
type AccountHandler struct {
	accountService *services.AccountService
	logger         utils.Logger
}

// NewAccountHandler creates a new account handler
func NewAccountHandler(accountService *services.AccountService, logger utils.Logger) *AccountHandler {
	return &AccountHandler{
		accountService: accountService,
		logger:         logger,
	}
}

// GetAccount handles GET /api/v1/accounts/:name
func (h *AccountHandler) GetAccount(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		h.respondWithError(c, http.StatusBadRequest, "Account name is required")
		return
	}

	account, err := h.accountService.GetAccount(c.Request.Context(), name)
	if err != nil {
		h.logger.Error("Failed to get account", utils.String("name", name), utils.Error(err))
		h.respondWithError(c, http.StatusNotFound, "Account not found")
		return
	}

	h.respondWithSuccess(c, account)
}

// GetAccounts handles GET /api/v1/accounts
func (h *AccountHandler) GetAccounts(c *gin.Context) {
	// Accepts both the canonical names (page_size/sort_by/sort_order) and
	// the short aliases (limit/sort/order), same convention as /v1/blocks.
	// `search` narrows the listing to account-name prefixes.
	params := parsePaginationParams(c)
	sortParams := parseSortParams(c, "reputation", "desc")
	search := c.Query("search")

	result, err := h.accountService.GetAccounts(c.Request.Context(), params, sortParams, search)
	if err != nil {
		h.logger.Error("Failed to get accounts", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve accounts")
		return
	}

	h.respondWithSuccessAndMeta(c, result.Accounts, &models.Meta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize)),
	})
}

// SearchAccounts handles GET /api/v1/accounts/search
func (h *AccountHandler) SearchAccounts(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		h.respondWithError(c, http.StatusBadRequest, "Search query is required")
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	result, err := h.accountService.SearchAccounts(c.Request.Context(), query, limit)
	if err != nil {
		h.logger.Error("Failed to search accounts", utils.String("query", query), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to search accounts")
		return
	}

	h.respondWithSuccess(c, result)
}

// GetAccountStats handles GET /api/v1/accounts/stats
func (h *AccountHandler) GetAccountStats(c *gin.Context) {
	stats, err := h.accountService.GetAccountStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get account stats", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve account statistics")
		return
	}

	h.respondWithSuccess(c, stats)
}

// GetTopAccounts handles GET /api/v1/accounts/top
func (h *AccountHandler) GetTopAccounts(c *gin.Context) {
	criteria := c.DefaultQuery("criteria", "reputation")
	limit := 50

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	accounts, err := h.accountService.GetTopAccounts(c.Request.Context(), criteria, limit)
	if err != nil {
		h.logger.Error("Failed to get top accounts", utils.String("criteria", criteria), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve top accounts")
		return
	}

	h.respondWithSuccess(c, accounts)
}

// GetAccountHistory handles GET /api/v1/accounts/:name/history
func (h *AccountHandler) GetAccountHistory(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		h.respondWithError(c, http.StatusBadRequest, "Account name is required")
		return
	}

	// Parse pagination parameters (page plus the page_size/limit alias
	// pair shared by all listings, params.go).
	params := parsePaginationParams(c)

	result, err := h.accountService.GetAccountHistory(c.Request.Context(), name, params)
	if err != nil {
		h.logger.Error("Failed to get account history", utils.String("name", name), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve account history")
		return
	}

	h.respondWithSuccessAndMeta(c, result.Operations, &models.Meta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: result.TotalPages,
	})
}

// Helper methods

func (h *AccountHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *AccountHandler) respondWithSuccessAndMeta(c *gin.Context, data interface{}, meta *models.Meta) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	})
}

func (h *AccountHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
