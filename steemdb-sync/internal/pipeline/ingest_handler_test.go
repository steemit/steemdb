package pipeline

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/mongo"
)

// createTestHandler creates a test handler with a real batcher
// Note: This requires MongoDB to be running. If MongoDB is not available,
// the tests will be skipped.
func createTestHandler(t *testing.T) *IngestHandler {
	cfg := &config.Config{
		Mongo: config.MongoConfig{
			URI:      "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin",
			Database: "steemdb_test",
		},
		Ingest: config.IngestConfig{
			QueueSize:  10000,
			ListenAddr: ":8080",
		},
		Batch: config.BatchConfig{
			Size:         1000,
			FlushInterval: "1s",
		},
	}

	// Try to create MongoDB client
	mongoClient, err := mongo.NewClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available for testing: %v", err)
		return nil
	}

	batcher, err := NewBatcher(cfg, mongoClient)
	require.NoError(t, err)
	batcher.Start()
	t.Cleanup(func() { batcher.Stop() })

	handler := NewIngestHandler(batcher)
	return handler
}

func TestHandleAppliedOps_EmptyArray(t *testing.T) {
	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	reqBody := []byte("[]")
	req := httptest.NewRequest("POST", "/ingest/applied_ops", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	// Error response is plain text, not JSON
	assert.Contains(t, w.Body.String(), "empty operations array")
}

func TestHandleAppliedOps_SingleOperation(t *testing.T) {
	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	operations := []OperationJSON{
		{
			Block: BlockJSON{
				Num:       1,
				ID:        "0000000000000001",
				Timestamp: "2016-03-24T16:05:00",
			},
			Transaction: TransactionJSON{
				ID:    stringPtr("test_trx_id"),
				Index: 0,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "transfer",
				Value: map[string]interface{}{
					"from":   "alice",
					"to":     "bob",
					"amount": "1.000 STEEM",
				},
			},
			Virtual: false,
		},
	}

	reqBody, err := json.Marshal(operations)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/ingest/applied_ops", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, 1, response.Processed)
	assert.Empty(t, response.Errors)
}

func TestHandleAppliedOps_MultipleOperations(t *testing.T) {
	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	operations := []OperationJSON{
		{
			Block: BlockJSON{
				Num:       1,
				ID:        "0000000000000001",
				Timestamp: "2016-03-24T16:05:00",
			},
			Transaction: TransactionJSON{
				ID:    stringPtr("trx1"),
				Index: 0,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "transfer",
				Value: map[string]interface{}{},
			},
			Virtual: false,
		},
		{
			Block: BlockJSON{
				Num:       1,
				ID:        "0000000000000001",
				Timestamp: "2016-03-24T16:05:00",
			},
			Transaction: TransactionJSON{
				ID:    nil,
				Index: -1,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "author_reward",
				Value: map[string]interface{}{},
			},
			Virtual: true,
		},
		{
			Block: BlockJSON{
				Num:       2,
				ID:        "0000000000000002",
				Timestamp: "2016-03-24T16:05:03",
			},
			Transaction: TransactionJSON{
				ID:    stringPtr("trx2"),
				Index: 0,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "vote",
				Value: map[string]interface{}{},
			},
			Virtual: false,
		},
	}

	reqBody, err := json.Marshal(operations)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/ingest/applied_ops", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, 3, response.Processed)
	assert.Empty(t, response.Errors)
}

func TestHandleAppliedOps_InvalidJSON(t *testing.T) {
	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	reqBody := []byte("invalid json")
	req := httptest.NewRequest("POST", "/ingest/applied_ops", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAppliedOps_WrongMethod(t *testing.T) {
	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	req := httptest.NewRequest("GET", "/ingest/applied_ops", nil)
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleAppliedOps_PartialFailure(t *testing.T) {
	// Note: This test demonstrates partial failure handling.
	// In practice, JSON decoding errors will fail the entire request (400 Bad Request).
	// Partial failures would occur if operations are valid JSON but fail during processing
	// (e.g., batcher queue full, MongoDB write errors). For now, we test that the
	// batch processing logic correctly handles multiple operations.

	handler := createTestHandler(t)
	if handler == nil {
		return
	}

	// Create valid operations - all should succeed
	operations := []OperationJSON{
		{
			Block: BlockJSON{
				Num:       1,
				ID:        "0000000000000001",
				Timestamp: "2016-03-24T16:05:00",
			},
			Transaction: TransactionJSON{
				ID:    stringPtr("trx1"),
				Index: 0,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "transfer",
				Value: map[string]interface{}{},
			},
			Virtual: false,
		},
		{
			Block: BlockJSON{
				Num:       2,
				ID:        "0000000000000002",
				Timestamp: "2016-03-24T16:05:03",
			},
			Transaction: TransactionJSON{
				ID:    stringPtr("trx2"),
				Index: 0,
			},
			Operation: OperationDataJSON{
				Index: 0,
				Type:  "vote",
				Value: map[string]interface{}{},
			},
			Virtual: false,
		},
	}

	reqBody, err := json.Marshal(operations)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/ingest/applied_ops", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAppliedOps(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, 2, response.Processed)
	assert.Empty(t, response.Errors)
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
