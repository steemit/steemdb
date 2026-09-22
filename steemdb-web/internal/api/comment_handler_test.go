package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// handlerTestLogger returns a quiet logger for handler tests.
func handlerTestLogger() utils.Logger {
	logger, err := utils.NewLogger(utils.LogConfig{Level: "error"})
	if err != nil {
		panic(err)
	}
	return logger
}

// performGetPosts runs the CommentHandler.GetPosts handler against a GET
// request with the given query string and reports (recordedStatus,
// reachedService). The handler is wired with a nil service: any call that
// passes the parameter guard reaches the nil service and panics, which is
// recovered and reported as reachedService=true — proving the guard, not
// the service, produced the recorded status.
func performGetPosts(t *testing.T, query string) (status int, reachedService bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/?"+query, nil)

	handler := NewCommentHandler(nil, handlerTestLogger())

	// A panic inside the handler would skip the return statement below, so
	// the deferred function owns the result when the service is reached.
	defer func() {
		if recover() != nil {
			reachedService = true
		}
		status = recorder.Code
	}()
	handler.GetPosts(c)
	return recorder.Code, false
}

// TestGetPostsRejectsSearch pins the explicit no-search contract of the
// posts listing: `search` has no semantics there (it only paginates and
// sorts the full depth=0 set), so it must fail loudly with 400 instead of
// being silently ignored.
func TestGetPostsRejectsSearch(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantStatus     int
		wantReachedSvc bool
	}{
		{
			name:           "search supplied is rejected before the service call",
			query:          "search=hello&page=1",
			wantStatus:     http.StatusBadRequest,
			wantReachedSvc: false,
		},
		{
			name:           "no search passes the guard and reaches the service",
			query:          "page=2&page_size=50",
			wantStatus:     http.StatusOK, // unused: status is only asserted when the service is NOT reached
			wantReachedSvc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reachedService := performGetPosts(t, tt.query)
			if reachedService != tt.wantReachedSvc {
				t.Errorf("reached service = %v, want %v", reachedService, tt.wantReachedSvc)
			}
			if !reachedService && status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
		})
	}
}
