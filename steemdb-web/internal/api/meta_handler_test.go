package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// doMetaRequest serves GET /api/v1/meta against the given (pre-Normalized)
// analytics config and returns status + body, mirroring the setup style in
// health_handler_test.go.
func doMetaRequest(t *testing.T, analytics *utils.AnalyticsConfig) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/meta", NewMetaHandler(analytics).GetMeta)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/meta", nil)
	router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// TestMetaOmitsAnalyticsWhenDisabled verifies tracking-off sends no
// "analytics" key at all (clients treat absent and "none" identically) —
// the SPA must not load a snippet it was not configured with.
func TestMetaOmitsAnalyticsWhenDisabled(t *testing.T) {
	code, body := doMetaRequest(t, &utils.AnalyticsConfig{Provider: "none"})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	if !strings.Contains(body, `"service":"steemdb-web"`) {
		t.Errorf("body missing service identity, got: %s", body)
	}
	if strings.Contains(body, "analytics") {
		t.Errorf("body must not contain an analytics key when disabled, got: %s", body)
	}
}

// TestMetaNilAnalytics is a regression guard: a nil config pointer (not
// expected in production — main passes &config.Analytics) must not panic.
func TestMetaNilAnalytics(t *testing.T) {
	code, body := doMetaRequest(t, nil)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	if strings.Contains(body, "analytics") {
		t.Errorf("body must not contain an analytics key, got: %s", body)
	}
}

// TestMetaServesGoogleAnalytics verifies the ga shape: provider + GA4
// measurement id, and no umami url field.
func TestMetaServesGoogleAnalytics(t *testing.T) {
	code, body := doMetaRequest(t, &utils.AnalyticsConfig{Provider: "ga", GAID: "G-ABC123"})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	for _, want := range []string{`"provider":"ga"`, `"id":"G-ABC123"`, `"analytics":`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q, got: %s", want, body)
		}
	}
	if strings.Contains(body, "url") {
		t.Errorf("ga body must not carry a url field, got: %s", body)
	}
}

// TestMetaServesUmami verifies the umami shape: provider + base url, and no
// ga id field.
func TestMetaServesUmami(t *testing.T) {
	code, body := doMetaRequest(t, &utils.AnalyticsConfig{Provider: "umami", UmamiURL: "https://umami.example.com"})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", code, body)
	}
	for _, want := range []string{`"provider":"umami"`, `"url":"https://umami.example.com"`, `"analytics":`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q, got: %s", want, body)
		}
	}
	if strings.Contains(body, `"id"`) {
		t.Errorf("umami body must not carry an id field, got: %s", body)
	}
}
