package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// LabsHandler handles labs-related HTTP requests
type LabsHandler struct {
	labsService *services.LabsService
	logger      utils.Logger
}

// NewLabsHandler creates a new labs handler
func NewLabsHandler(labsService *services.LabsService, logger utils.Logger) *LabsHandler {
	return &LabsHandler{
		labsService: labsService,
		logger:      logger,
	}
}

// GetLabsIndex handles GET /api/v1/labs
func (h *LabsHandler) GetLabsIndex(c *gin.Context) {
	// Return list of available labs features
	labs := map[string]interface{}{
		"features": []string{
			"powerup",
			"powerdown",
			"rshares",
			"curation",
			"author",
			"flags",
			"clients",
			"benefactors",
			"pending",
		},
	}

	h.respondWithSuccess(c, labs)
}

// GetPowerUps handles GET /api/v1/labs/powerup
func (h *LabsHandler) GetPowerUps(c *gin.Context) {
	filter := c.DefaultQuery("filter", "")

	powerUps, err := h.labsService.GetPowerUps(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to get power ups", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve power ups")
		return
	}

	h.respondWithSuccess(c, powerUps)
}

// GetPowerDowns handles GET /api/v1/labs/powerdown
func (h *LabsHandler) GetPowerDowns(c *gin.Context) {
	powerDowns, err := h.labsService.GetPowerDowns(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get power downs", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve power downs")
		return
	}

	h.respondWithSuccess(c, powerDowns)
}

// GetRshares handles GET /api/v1/labs/rshares
func (h *LabsHandler) GetRshares(c *gin.Context) {
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	rshares, err := h.labsService.GetRsharesAllocation(c.Request.Context(), date)
	if err != nil {
		h.logger.Error("Failed to get rshares allocation", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve rshares allocation")
		return
	}

	h.respondWithSuccess(c, rshares)
}

// GetCuration handles GET /api/v1/labs/curation
func (h *LabsHandler) GetCuration(c *gin.Context) {
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	grouping := c.DefaultQuery("grouping", "daily")

	var date time.Time
	var err error

	if grouping == "monthly" {
		dateStr = c.DefaultQuery("date", time.Now().Format("2006-01"))
		date, err = time.Parse("2006-01", dateStr)
	} else {
		date, err = time.Parse("2006-01-02", dateStr)
	}

	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format")
		return
	}

	leaderboard, err := h.labsService.GetCurationLeaderboard(c.Request.Context(), date, grouping)
	if err != nil {
		h.logger.Error("Failed to get curation leaderboard", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve curation leaderboard")
		return
	}

	h.respondWithSuccess(c, leaderboard)
}

// GetAuthor handles GET /api/v1/labs/author
func (h *LabsHandler) GetAuthor(c *gin.Context) {
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	grouping := c.DefaultQuery("grouping", "daily")

	var date time.Time
	var err error

	if grouping == "monthly" {
		dateStr = c.DefaultQuery("date", time.Now().Format("2006-01"))
		date, err = time.Parse("2006-01", dateStr)
	} else {
		date, err = time.Parse("2006-01-02", dateStr)
	}

	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format")
		return
	}

	leaderboard, err := h.labsService.GetAuthorLeaderboard(c.Request.Context(), date, grouping)
	if err != nil {
		h.logger.Error("Failed to get author leaderboard", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve author leaderboard")
		return
	}

	h.respondWithSuccess(c, leaderboard)
}

// GetFlags handles GET /api/v1/labs/flags
func (h *LabsHandler) GetFlags(c *gin.Context) {
	flags, err := h.labsService.GetFlags(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get flags", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve flags")
		return
	}

	h.respondWithSuccess(c, flags)
}

// GetClients handles GET /api/v1/labs/clients
func (h *LabsHandler) GetClients(c *gin.Context) {
	clients, err := h.labsService.GetClients(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get clients", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve clients")
		return
	}

	h.respondWithSuccess(c, clients)
}

// GetBenefactors handles GET /api/v1/labs/benefactors
func (h *LabsHandler) GetBenefactors(c *gin.Context) {
	benefactors, err := h.labsService.GetBenefactors(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get benefactors", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve benefactors")
		return
	}

	h.respondWithSuccess(c, benefactors)
}

// GetPending handles GET /api/v1/labs/pending
func (h *LabsHandler) GetPending(c *gin.Context) {
	pending, err := h.labsService.GetPendingPosts(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get pending posts", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve pending posts")
		return
	}

	h.respondWithSuccess(c, pending)
}

// Helper methods
func (h *LabsHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      data,
		"timestamp": time.Now(),
	})
}

func (h *LabsHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"success": false,
		"error": gin.H{
			"code":    statusCode,
			"message": message,
		},
		"timestamp": time.Now(),
	})
}
