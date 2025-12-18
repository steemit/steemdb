package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// LegacyHandler handles legacy API endpoint redirects
type LegacyHandler struct {
	steemClient *steem.Client
	logger      utils.Logger
}

// NewLegacyHandler creates a new legacy handler
func NewLegacyHandler(steemClient *steem.Client, logger utils.Logger) *LegacyHandler {
	return &LegacyHandler{
		steemClient: steemClient,
		logger:      logger,
	}
}

// RedirectToV1 redirects legacy API endpoints to new /api/v1 paths
func (h *LegacyHandler) RedirectToV1(c *gin.Context) {
	// Extract path from URL
	path := c.Request.URL.Path
	// Remove "/api/" prefix
	if len(path) > 5 && path[:5] == "/api/" {
		path = path[5:]
	}

	// Map legacy endpoints to new v1 endpoints
	redirectMap := map[string]string{
		"supply":       "/api/v1/labs/powerup",  // Currency supply - redirecting to powerup for now
		"props":        "/api/v1/dashboard",     // Global props history
		"percentage":   "/api/v1/dashboard",     // Percentage vesting - can be calculated from dashboard props
		"rshares":      "/api/v1/labs/rshares",  // Voter rshares
		"downvotes":    "/api/v1/labs/flags",    // Downvoters
		"topwitnesses": "/api/v1/witnesses",     // Top 50 witnesses voters
		"rewards":      "/api/v1/labs/author",   // Daily Author Rewards (90-day)
		"curation":     "/api/v1/labs/curation", // Daily Curation Rewards (90-day)
		"powerup":      "/api/v1/labs/powerup",  // STEEM -> VESTS per Day
		"steem":        "/api/v1/labs/powerup",  // STEEM -> VESTS per Day (alternative endpoint)
	}

	newPath, exists := redirectMap[path]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Legacy endpoint not found",
			"path":  path,
		})
		return
	}

	// Preserve query parameters
	query := c.Request.URL.RawQuery
	if query != "" {
		newPath += "?" + query
	}

	// 302 redirect
	c.Redirect(http.StatusFound, newPath)
}

// GetToken handles GET /api/token?coin=steem|sbd|sp
// Returns plain text token supply amount
func (h *LegacyHandler) GetToken(c *gin.Context) {
	coin := strings.ToLower(c.Query("coin"))

	// Valid coins: steem, sbd, sp
	validCoins := map[string]bool{
		"steem": true,
		"sbd":   true,
		"sp":    true,
	}

	if !validCoins[coin] {
		c.String(http.StatusOK, "")
		return
	}

	// Get blockchain properties
	props, err := h.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		h.logger.Error("Failed to get dynamic global properties", utils.Error(err))
		c.String(http.StatusInternalServerError, "")
		return
	}

	var value float64
	switch coin {
	case "steem":
		// Parse current_supply (format: "1234567.123 STEEM")
		parts := strings.Fields(props.CurrentSupply)
		if len(parts) > 0 {
			if v, err := parseFloat(parts[0]); err == nil {
				value = v
			} else {
				h.logger.Error("Failed to parse current_supply", utils.String("supply", props.CurrentSupply), utils.Error(err))
				c.String(http.StatusOK, "")
				return
			}
		} else {
			c.String(http.StatusOK, "")
			return
		}
	case "sbd":
		// Parse current_sbd_supply (format: "1234567.123 SBD")
		parts := strings.Fields(props.CurrentSBDSupply)
		if len(parts) > 0 {
			if v, err := parseFloat(parts[0]); err == nil {
				value = v
			} else {
				h.logger.Error("Failed to parse current_sbd_supply", utils.String("supply", props.CurrentSBDSupply), utils.Error(err))
				c.String(http.StatusOK, "")
				return
			}
		} else {
			c.String(http.StatusOK, "")
			return
		}
	case "sp":
		// For SP, legacy code doesn't implement it, so return empty
		c.String(http.StatusOK, "")
		return
	default:
		c.String(http.StatusOK, "")
		return
	}

	// Format to 3 decimal places and return as plain text
	result := fmt.Sprintf("%.3f", value)
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, result)
}

// Helper function to parse float from string
func parseFloat(s string) (float64, error) {
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}
