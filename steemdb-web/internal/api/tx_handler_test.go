package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// doTxRequest serves GET /api/v1/tx/:id against a handler with a nil tx
// service — only paths that never reach the service (ID validation) are
// under test here.
func doTxRequest(t *testing.T, id string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/tx/:id", NewTxHandler(nil, nil).GetTx)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tx/"+id, nil)
	router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// TestGetTxRejectsInvalidID verifies the handler validates the 40-hex
// transaction ID shape before touching the upstream RPC.
func TestGetTxRejectsInvalidID(t *testing.T) {
	invalid := []string{
		"not-a-transaction-id",
		"0123456789abcdef0123456789abcdef0123456",   // 39 hex chars
		"0123456789abcdef0123456789abcdef012345678", // 41 hex chars
		"zz3456789abcdef0123456789abcdef0123456789", // 40 chars, not hex
	}
	for _, id := range invalid {
		code, body := doTxRequest(t, id)
		if code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d (body: %s)", id, code, body)
		}
		if !strings.Contains(body, "Invalid transaction ID") {
			t.Errorf("id %q: body missing error message, got: %s", id, body)
		}
	}
}

// TestIsTxNotFoundError verifies steemd's missing-transaction errors are told
// apart from connectivity failures.
func TestIsTxNotFoundError(t *testing.T) {
	notFound := []string{
		"RPC call failed after 4 attempts: unknown transaction",
		"Assert Exception:false: Unknown Transaction 1111111111111111111111111111111111111111",
		"Assert Exception:blk->transactions.size() > txInBlock: ",
	}
	for _, msg := range notFound {
		if !isTxNotFoundError(errors.New(msg)) {
			t.Errorf("expected %q to be classified as not-found", msg)
		}
	}
	if isTxNotFoundError(errors.New("RPC call failed after 4 attempts: steem rpc: attempt timed out")) {
		t.Error("expected timeout error NOT to be classified as not-found")
	}
}
