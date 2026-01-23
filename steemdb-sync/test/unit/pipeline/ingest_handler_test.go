package pipeline_test

import (
	"context"
	"testing"

	"github.com/steemit/steemdb-sync/internal/pipeline"
	"github.com/stretchr/testify/assert"
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
