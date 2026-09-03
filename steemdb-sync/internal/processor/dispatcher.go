package processor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/model"
)

// Dispatcher routes operations to handlers by op_type.
// It is safe for concurrent use once registered.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]OpHandler
	ctx      *Context
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(ctx *Context) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]OpHandler),
		ctx:      ctx,
	}
}

// Register associates an op_type with a handler.
// If called after Start, behavior is undefined — register all handlers before processing.
func (d *Dispatcher) Register(opType string, handler OpHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[opType] = handler
}

// Dispatch sends an operation to its registered handler.
// If no handler is registered for the op_type, it is silently skipped (debug log).
func (d *Dispatcher) Dispatch(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	d.mu.RLock()
	handler, ok := d.handlers[op.OpType]
	d.mu.RUnlock()

	if !ok {
		// Unknown op_type — skip silently. This matches legacy behavior (only ~14 op_types handled).
		return nil
	}

	err := safeHandle(ctx, handler, op, blockTS)
	if err != nil {
		return errors.Wrapf(err, "handler error for op_type=%s id=%s", op.OpType, op.ID)
	}

	return nil
}

// safeHandle invokes a handler and converts panics into errors. A panicking
// handler must degrade to that op's failure — DispatchBlock's contract that
// one bad op never stops the block (or the process) would otherwise be void.
func safeHandle(ctx context.Context, handler OpHandler, op *model.Operation, blockTS time.Time) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panicked: %v", r)
		}
	}()
	return handler.Handle(ctx, op, blockTS)
}

// DispatchBlock sends all operations in a block to their handlers in order.
// Operations are dispatched sequentially to preserve intra-block ordering semantics
// (e.g., a comment op must precede a vote op on that comment within the same block).
// A single op failure does not stop the rest of the block (matches legacy sync.py).
func (d *Dispatcher) DispatchBlock(ctx context.Context, ops []*model.Operation, blockTS time.Time) (errors int) {
	for _, op := range ops {
		if err := d.Dispatch(ctx, op, blockTS); err != nil {
			log.Printf("[Processor] Error processing op %s (type=%s, block=%d): %v",
				op.ID, op.OpType, op.BlockNum, err)
			errors++
			// continue — don't let one bad op stop the whole block
		}
	}
	return errors
}

// RegisteredTypes returns the set of registered op_types (for logging on startup).
func (d *Dispatcher) RegisteredTypes() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	types := make([]string, 0, len(d.handlers))
	for t := range d.handlers {
		types = append(types, t)
	}
	return types
}
