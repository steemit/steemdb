package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// CORSMiddleware handles cross-origin requests per the api.cors config.
// Reflects the request Origin when it appears in allowed_origins (or "*"),
// answers preflight OPTIONS with 204, and passes everything else through.
func CORSMiddleware(cfg utils.CORSConfig) gin.HandlerFunc {
	allowAll := false
	allowed := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}
	headers := cfg.AllowedHeaders
	if len(headers) == 0 {
		headers = []string{"Content-Type", "Authorization"}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || !(allowAll || allowed[origin]) {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(headers, ", "))
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
