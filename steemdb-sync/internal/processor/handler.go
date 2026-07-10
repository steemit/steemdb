package processor

import (
	"context"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
)

// OpHandler processes a single operation and writes derived data to MongoDB.
// op.OpValue is a map[string]interface{} equivalent to the legacy Python op dict.
// blockTS is the timestamp of the block this operation belongs to.
//
// Handlers MUST:
//   - Be idempotent (all writes use upsert)
//   - Not panic on malformed input (return error instead)
//   - Not block excessively (the main loop calls them sequentially)
type OpHandler interface {
	Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error
}

// OpHandlerFunc is a function adapter for OpHandler.
type OpHandlerFunc func(ctx context.Context, op *model.Operation, blockTS time.Time) error

func (f OpHandlerFunc) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	return f(ctx, op, blockTS)
}
