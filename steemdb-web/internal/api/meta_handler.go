package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// MetaHandler serves the SPA bootstrap endpoint GET /api/v1/meta — the
// client-side app config the frontend fetches on boot.
//
// It deliberately carries only the analytics selection (provider +
// credential), not any other server state: the web service keeps NO
// analytics snippet in the static HTML (the previous mechanism
// sed-replaced a __GA_ID__ placeholder in index.html on every container
// start), so the SPA loads the matching third-party script on the client
// after this call. Adding a provider is a config change, never a
// deployment-pipeline change.
type MetaHandler struct {
	analytics *utils.AnalyticsConfig
}

// NewMetaHandler creates a new meta handler
func NewMetaHandler(analytics *utils.AnalyticsConfig) *MetaHandler {
	return &MetaHandler{analytics: analytics}
}

// GetMeta handles GET /api/v1/meta.
//
// The response is small and vendor-neutral. `analytics` is omitted (rather
// than sent as {"provider":"none"}) when tracking is disabled, so a client
// treats "absent" and "none" identically. The GA measurement ID is public
// by design (Google Analytics 4 IDs are not secrets); the Umami base URL is
// only sent when an absolute http(s) URL is configured — validation happens
// in (*utils.AnalyticsConfig).Normalize, which fails closed to "none".
func (h *MetaHandler) GetMeta(c *gin.Context) {
	resp := gin.H{
		"service": "steemdb-web",
		"version": "1.0.0",
	}
	if a := h.analytics; a != nil && a.Provider != "none" {
		analytics := gin.H{"provider": a.Provider}
		if a.Provider == "ga" {
			analytics["id"] = a.GAID
		} else {
			analytics["url"] = a.UmamiURL
		}
		resp["analytics"] = analytics
	}
	c.JSON(http.StatusOK, resp)
}
