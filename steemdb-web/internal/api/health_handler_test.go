package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakePinger stands in for *database.MongoDB in health-handler tests.
type fakePinger struct {
	err error
}

func (f fakePinger) Ping(ctx context.Context) error { return f.err }

func setupHealthRouter(p Pinger) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewHealthHandler(p)
	router.GET("/health", h.Health)
	router.GET("/ready", h.Ready)
	return router
}

// TestReadyChecksOnlyMongo verifies /ready is ready exactly when the Mongo
// ping succeeds — Redis is no longer part of readiness (it was removed from
// the service entirely), so Mongo is the single dependency probed.
func TestReadyChecksOnlyMongo(t *testing.T) {
	router := setupHealthRouter(fakePinger{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ready", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for healthy Mongo, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestReadyFailsWhenMongoUnavailable(t *testing.T) {
	router := setupHealthRouter(fakePinger{err: errors.New("connection refused")})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ready", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for failed Mongo ping, got %d (body: %s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "database_unavailable") {
		t.Errorf("expected database_unavailable error in body, got: %s", body)
	}
}

// TestHealthChecksNoDependency verifies /health stays pure liveness: it must
// report healthy even when the database ping fails.
func TestHealthChecksNoDependency(t *testing.T) {
	router := setupHealthRouter(fakePinger{err: errors.New("connection refused")})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from liveness even with db down, got %d (body: %s)", w.Code, w.Body.String())
	}
}
