package processor

import (
	"context"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
)

// makeTestOp creates an Operation with the given type and value map.
func makeTestOp(opType string, blockNum uint32) *model.Operation {
	return &model.Operation{
		ID:       "100:0:0",
		BlockNum: blockNum,
		TrxID:    "trx123",
		OpIndex:  0,
		OpType:   opType,
		OpValue: map[string]interface{}{
			"author": "alice",
		},
		Virtual: false,
	}
}

func TestDispatcherRegisterAndDispatch(t *testing.T) {
	ctx := &Context{}
	d := NewDispatcher(ctx)

	called := false
	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		called = true
		if op.OpType != "vote" {
			t.Errorf("expected op_type vote, got %s", op.OpType)
		}
		return nil
	}))

	op := makeTestOp("vote", 100)
	if err := d.Dispatch(context.Background(), op, time.Now()); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	if !called {
		t.Error("handler was not called")
	}
}

func TestDispatcherUnknownOpTypeSkipped(t *testing.T) {
	ctx := &Context{}
	d := NewDispatcher(ctx)

	// Register a handler for "vote" only
	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		t.Error("vote handler should not be called for unknown op_type")
		return nil
	}))

	// Dispatch an unregistered op_type — should not error, not call any handler
	op := makeTestOp("some_unknown_type", 100)
	if err := d.Dispatch(context.Background(), op, time.Now()); err != nil {
		t.Fatalf("Dispatch for unknown type should not error, got: %v", err)
	}
}

func TestDispatcherHandlerError(t *testing.T) {
	ctx := &Context{}
	d := NewDispatcher(ctx)

	d.Register("transfer", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		return errSentinel
	}))

	op := makeTestOp("transfer", 100)
	if err := d.Dispatch(context.Background(), op, time.Now()); err == nil {
		t.Fatal("expected error from handler, got nil")
	}
}

func TestDispatchBlockSequentialAndErrorCount(t *testing.T) {
	ctx := &Context{}
	d := NewDispatcher(ctx)

	callOrder := []int32{}
	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		callOrder = append(callOrder, op.OpIndex)
		return nil
	}))
	d.Register("bad", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		callOrder = append(callOrder, op.OpIndex)
		return errSentinel
	}))

	ops := []*model.Operation{
		{ID: "100:0:0", BlockNum: 100, OpIndex: 0, OpType: "vote", OpValue: map[string]interface{}{}},
		{ID: "100:0:1", BlockNum: 100, OpIndex: 1, OpType: "bad", OpValue: map[string]interface{}{}},
		{ID: "100:0:2", BlockNum: 100, OpIndex: 2, OpType: "vote", OpValue: map[string]interface{}{}},
		{ID: "100:0:3", BlockNum: 100, OpIndex: 3, OpType: "unknown", OpValue: map[string]interface{}{}},
	}

	errCount := d.DispatchBlock(context.Background(), ops, time.Now())

	// Should process all in order despite the error on op index 1
	if len(callOrder) != 3 { // 3 ops with registered handlers; unknown skipped
		t.Errorf("expected 3 handler calls, got %d (order: %v)", len(callOrder), callOrder)
	}
	if callOrder[0] != 0 || callOrder[1] != 1 || callOrder[2] != 2 {
		t.Errorf("ops not processed in order, got: %v", callOrder)
	}
	if errCount != 1 {
		t.Errorf("expected 1 error, got %d", errCount)
	}
}

func TestRegisteredTypes(t *testing.T) {
	ctx := &Context{}
	d := NewDispatcher(ctx)

	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error { return nil }))
	d.Register("transfer", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error { return nil }))

	types := d.RegisteredTypes()
	if len(types) != 2 {
		t.Errorf("expected 2 registered types, got %d: %v", len(types), types)
	}
}
