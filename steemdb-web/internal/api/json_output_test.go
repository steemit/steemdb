package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestJSONResponsesEscapeHTML pins the API half of the output-escaping
// contract: every string served by steemdb-web comes from the chain (post
// bodies, titles, memo keys, profile metadata — whoever wrote it controls
// the bytes), and it must leave the service as JSON with <, > and &
// escaped. The SPA renders those strings as React text, so this is the
// second layer: a client that inlines a response into an HTML document
// (an <script> block, a legacy template) still cannot break out of it.
//
// If this test fails, someone switched the renderer to a non-escaping JSON
// encoder (jsoniter, c.PureJSON, SetEscapeHTML(false)) — see
// docs/AI-driver/04-web-service.md, "Output escaping".
func TestJSONResponsesEscapeHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"body":  `<script>alert("xss")</script> & '</div>'`,
				"title": `<img src=x onerror=alert(1)>`,
			},
		})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q, want application/json", ct)
	}

	raw := w.Body.String()
	if strings.Contains(raw, "<script>") || strings.Contains(raw, "<img") {
		t.Errorf("raw markup left unescaped in response: %s", raw)
	}
	for _, want := range []string{`\u003cscript\u003e`, `\u003cimg`, `\u0026`} {
		if !strings.Contains(raw, want) {
			t.Errorf("response missing escaped sequence %q: %s", want, raw)
		}
	}

	// Escaping must not corrupt the payload: clients decode the original
	// bytes back, otherwise post bodies would render with mangled markup.
	var parsed struct {
		Data struct {
			Body  string `json:"body"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, want := parsed.Data.Body, `<script>alert("xss")</script> & '</div>'`; got != want {
		t.Errorf("body round-trip = %q, want %q", got, want)
	}
	if got, want := parsed.Data.Title, `<img src=x onerror=alert(1)>`; got != want {
		t.Errorf("title round-trip = %q, want %q", got, want)
	}
}
