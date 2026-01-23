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

// Note: Single endpoint tests removed - we now only use batch endpoint /ingest/applied_ops
// All operations are sent as arrays, even single operations: [{...}]
