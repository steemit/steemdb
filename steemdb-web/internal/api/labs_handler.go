package api

import (
	"net/http"
	"strconv"
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
	dateStr := c.DefaultQuery("date", time.Now().UTC().Format("2006-01-02"))

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	// The board is capped and ships its own totals; percentages must be derived
	// from those totals, not from the (truncated) array.
	board, err := h.labsService.GetRsharesAllocation(c.Request.Context(), date, parseLabsLimit(c))
	if err != nil {
		h.logger.Error("Failed to get rshares allocation", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve rshares allocation")
		return
	}

	h.respondWithSuccess(c, board)
}

// parseLabsLimit reads the optional `limit` row cap. 0 means "server default";
// the service clamps to its own maximum.
func parseLabsLimit(c *gin.Context) int {
	raw := c.Query("limit")
	if raw == "" {
		return 0
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return 0
	}
	return limit
}

// parseLeaderboardDate resolves the `date` query value for the curation and
// author leaderboards.
//
// grouping=monthly accepts YYYY-MM and YYYY-MM-DD: a date control that keeps
// its daily value while the grouping switches to monthly used to 400 here.
// The fallback default is the current UTC day/month (the aggregation windows
// are UTC), never the server-local date, so a request without `date` cannot
// land on the neighbouring day because of the container's TZ.
func parseLeaderboardDate(grouping, dateStr string) (time.Time, error) {
	if dateStr == "" {
		if grouping == "monthly" {
			dateStr = time.Now().UTC().Format("2006-01")
		} else {
			dateStr = time.Now().UTC().Format("2006-01-02")
		}
	}

	if grouping == "monthly" {
		if date, err := time.Parse("2006-01", dateStr); err == nil {
			return date, nil
		}
		return time.Parse("2006-01-02", dateStr)
	}
	return time.Parse("2006-01-02", dateStr)
}

// GetCuration handles GET /api/v1/labs/curation
func (h *LabsHandler) GetCuration(c *gin.Context) {
	grouping := c.DefaultQuery("grouping", "daily")
	if grouping != "daily" && grouping != "monthly" {
		// Unknown groupings used to fall through to the daily window, so a
		// client asking for something else silently got a different board.
		h.respondWithError(c, http.StatusBadRequest, "Invalid grouping (daily or monthly)")
		return
	}

	date, err := parseLeaderboardDate(grouping, c.DefaultQuery("date", ""))
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
	grouping := c.DefaultQuery("grouping", "daily")
	if grouping != "daily" && grouping != "monthly" {
		h.respondWithError(c, http.StatusBadRequest, "Invalid grouping (daily or monthly)")
		return
	}

	date, err := parseLeaderboardDate(grouping, c.DefaultQuery("date", ""))
	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format")
		return
	}

	leaderboard, err := h.labsService.GetAuthorLeaderboard(c.Request.Context(), date, grouping, parseLabsLimit(c))
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
	// The per-day series is opt-in: it is the bulk of the payload and the SPA
	// renders only the per-client totals.
	includeDaily := c.Query("include") == "daily" || c.Query("daily") == "true"

	clients, err := h.labsService.GetClients(c.Request.Context(), includeDaily)
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
