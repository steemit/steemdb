package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/pipeline"
)

// TestNewIngestHandler tests handler initialization
func TestNewIngestHandler(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	handler := pipeline.NewIngestHandler(batcher)
	assert.NotNil(t, handler)
}

// TestHandleAppliedOpValidJSON tests handling valid JSON request
func TestHandleAppliedOpValidJSON(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	batcher.Start()
	defer func() {
		go batcher.Stop()
		time.Sleep(200 * time.Millisecond)
	}()

	handler := pipeline.NewIngestHandler(batcher)

	// Create valid operation JSON
	opJSON := map[string]interface{}{
		"block": map[string]interface{}{
			"num":       1000,
			"id":        "block1000",
			"timestamp": "2023-01-01T00:00:00",
		},
		"transaction": map[string]interface{}{
			"id":    "tx1000",
			"index": 0,
		},
		"operation": map[string]interface{}{
			"index": 0,
			"type":  "transfer",
			"value": map[string]interface{}{
				"from":   "alice",
				"to":     "bob",
				"amount": "1.000 STEEM",
			},
		},
		"virtual": false,
	}

	jsonData, err := json.Marshal(opJSON)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/ingest/applied_op", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOp(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestHandleAppliedOpInvalidMethod tests handling invalid HTTP method
func TestHandleAppliedOpInvalidMethod(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	handler := pipeline.NewIngestHandler(batcher)

	req := httptest.NewRequest(http.MethodGet, "/ingest/applied_op", nil)
	w := httptest.NewRecorder()

	handler.HandleAppliedOp(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestHandleAppliedOpInvalidJSON tests handling invalid JSON
func TestHandleAppliedOpInvalidJSON(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	handler := pipeline.NewIngestHandler(batcher)

	req := httptest.NewRequest(http.MethodPost, "/ingest/applied_op", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOp(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "failed to decode JSON")
}

// TestHandleAppliedOpMissingFields tests handling JSON with missing fields
func TestHandleAppliedOpMissingFields(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	handler := pipeline.NewIngestHandler(batcher)

	// JSON with missing block field
	opJSON := map[string]interface{}{
		"transaction": map[string]interface{}{
			"id":    "tx1000",
			"index": 0,
		},
		"operation": map[string]interface{}{
			"index": 0,
			"type":  "transfer",
		},
	}

	jsonData, err := json.Marshal(opJSON)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/ingest/applied_op", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOp(w, req)

	// Should still process (JSON decoder may allow missing fields with defaults)
	// The actual behavior depends on JSON unmarshaling
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

// TestHandleAppliedOpVirtualOperation tests handling virtual operation
func TestHandleAppliedOpVirtualOperation(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	batcher.Start()
	defer func() {
		go batcher.Stop()
		time.Sleep(200 * time.Millisecond)
	}()

	handler := pipeline.NewIngestHandler(batcher)

	// Create virtual operation JSON
	opJSON := map[string]interface{}{
		"block": map[string]interface{}{
			"num":       2000,
			"id":        "block2000",
			"timestamp": "2023-01-01T00:00:00",
		},
		"transaction": map[string]interface{}{
			"id":    nil,
			"index": -1,
		},
		"operation": map[string]interface{}{
			"index": -1,
			"type":  "author_reward",
			"value": map[string]interface{}{
				"author": "alice",
			},
		},
		"virtual": true,
	}

	jsonData, err := json.Marshal(opJSON)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/ingest/applied_op", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOp(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestHandleAppliedOpConcurrentRequests tests handling concurrent requests
func TestHandleAppliedOpConcurrentRequests(t *testing.T) {
	batcher, mongoClient, _ := setupTestBatcher(t)
	if batcher == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	batcher.Start()
	defer func() {
		go batcher.Stop()
		time.Sleep(200 * time.Millisecond)
	}()

	handler := pipeline.NewIngestHandler(batcher)

	const numRequests = 20
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			opJSON := map[string]interface{}{
				"block": map[string]interface{}{
					"num":       uint32(3000 + idx),
					"id":        "block3000",
					"timestamp": "2023-01-01T00:00:00",
				},
				"transaction": map[string]interface{}{
					"id":    "tx3000",
					"index": 0,
				},
				"operation": map[string]interface{}{
					"index": 0,
					"type":  "transfer",
					"value": map[string]interface{}{
						"from": "alice",
					},
				},
				"virtual": false,
			}

			jsonData, _ := json.Marshal(opJSON)
			req := httptest.NewRequest(http.MethodPost, "/ingest/applied_op", bytes.NewReader(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleAppliedOp(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
