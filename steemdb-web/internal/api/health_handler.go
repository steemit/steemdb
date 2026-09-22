package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger is the readiness dependency of the web service. *database.MongoDB
// satisfies it; the interface exists so tests can substitute a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves the process-level health endpoints (/health, /ready).
type HealthHandler struct {
	db Pinger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health handles GET /health: pure liveness. It checks no dependency — a
// hung MongoDB must not make an alive process look dead to a liveness probe.
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

// Ready handles GET /ready: the process is ready to serve traffic when
// MongoDB answers a ping. Redis used to be pinged here as well; that client
// was removed because nothing else used it (no cache, no rate limiter), so a
// Redis outage took web out of rotation behind a dependency it did not
// actually have. Readiness deliberately stays Mongo-only until a real
// Redis-backed feature exists.
func (h *HealthHandler) Ready(c *gin.Context) {
	if err := h.db.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"error":  "database_unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"timestamp": time.Now().Unix(),
	})
}
