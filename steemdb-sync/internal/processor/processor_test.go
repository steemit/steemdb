package processor

import (
	"context"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
)

// The tests below cover the window-planning and cursor-holding semantics of
// the processing loop (sync review finding F1):
//
//   - head gap: block header at the window start missing while later blocks
//     exist — the window must hold (no dispatch, no cursor advance);
//   - mid gap: gap inside the window — process up to the gap, then advance;
//   - ops present but header missing — hold instead of dispatching with a
//     zero timestamp;
//   - normal window — dispatch everything, advance to the contiguous end.
//
// They run against fakes of the windowStore / cursorStore abstractions; the
// production mongoWindowStore and the FlushAll/Advance error paths need a
// live MongoDB and are covered by the e2e suite instead.

// fakeWindowStore serves canned window metadata and operations and records
// the ranges it was asked for.
type fakeWindowStore struct {
	blocks []windowBlockMeta
	ops    []*model.Operation

	blocksErr error
	opsErr    error

	// opsIgnoreRange makes WindowOps return every canned op regardless of
	// the queried range, simulating an inconsistent snapshot (ops visible
	// for a block whose header is not). The production store is
	// range-filtered, so this is only used to exercise the pre-dispatch
	// guard's wiring.
	opsIgnoreRange bool

	opsQueries  [][2]uint32 // (start, end) ranges passed to WindowOps
	blocksCalls int
}

func (f *fakeWindowStore) WindowBlocks(ctx context.Context, start, end uint32) ([]windowBlockMeta, error) {
	f.blocksCalls++
	if f.blocksErr != nil {
		return nil, f.blocksErr
	}
	var out []windowBlockMeta
	for _, m := range f.blocks {
		if m.BlockNum >= start && m.BlockNum <= end {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeWindowStore) WindowOps(ctx context.Context, start, end uint32) ([]*model.Operation, error) {
	f.opsQueries = append(f.opsQueries, [2]uint32{start, end})
	if f.opsErr != nil {
		return nil, f.opsErr
	}
	if f.opsIgnoreRange {
		return f.ops, nil
	}
	var out []*model.Operation
	for _, op := range f.ops {
		if op.BlockNum >= start && op.BlockNum <= end {
			out = append(out, op)
		}
	}
	return out, nil
}

// fakeCursor records cursor advances.
type fakeCursor struct {
	height   uint32
	advanced []uint32
	getErr   error
	advErr   error
}

func (f *fakeCursor) Get(ctx context.Context) (uint32, error) { return f.height, f.getErr }

func (f *fakeCursor) Advance(ctx context.Context, blockNum uint32) error {
	if f.advErr != nil {
		return f.advErr
	}
	f.advanced = append(f.advanced, blockNum)
	f.height = blockNum
	return nil
}

// dispatchRecorder captures (op id, block timestamp) pairs in dispatch order.
type dispatchRecorder struct {
	calls []dispatchCall
}

type dispatchCall struct {
	op       *model.Operation
	blockTS  time.Time
	blockNum uint32
}

// register attaches a recording handler for every opType used in a test.
func (r *dispatchRecorder) register(d *Dispatcher, opTypes ...string) {
	for _, t := range opTypes {
		t := t
		d.Register(t, OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
			r.calls = append(r.calls, dispatchCall{op: op, blockTS: blockTS, blockNum: op.BlockNum})
			return nil
		}))
	}
}

// meta is a test helper for block metadata.
func meta(blockNum uint32, ts time.Time) windowBlockMeta {
	return windowBlockMeta{BlockNum: blockNum, Timestamp: ts}
}

// op is a test helper for operations.
func op(blockNum uint32, trxIndex, opIndex int32) *model.Operation {
	return &model.Operation{
		ID:       model.OperationID(blockNum, trxIndex, opIndex),
		BlockNum: blockNum,
		TrxIndex: trxIndex,
		OpIndex:  opIndex,
		OpType:   "vote",
		OpValue:  map[string]interface{}{},
	}
}

// newTestProcessor wires a Processor over fakes (no MongoDB, no inserter —
// FlushAll is skipped for a nil inserter, matching production's guard).
func newTestProcessor(store windowStore, cur cursorStore, dispatcher *Dispatcher) *Processor {
	return &Processor{
		cfg:        &config.Config{},
		dispatcher: dispatcher,
		cursor:     cur,
		inserter:   nil,
		store:      store,
	}
}

func TestPlanWindow(t *testing.T) {
	const start = uint32(101)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		blocks []windowBlockMeta
		want   uint32
	}{
		{
			name:   "empty window waits (effective end below start)",
			blocks: nil,
			want:   start - 1,
		},
		{
			name:   "head block missing while later blocks exist waits",
			blocks: []windowBlockMeta{meta(start+1, ts), meta(start+2, ts)},
			want:   start - 1,
		},
		{
			name:   "single head block processes exactly the head",
			blocks: []windowBlockMeta{meta(start, ts)},
			want:   start,
		},
		{
			name:   "contiguous blocks process through the last one",
			blocks: []windowBlockMeta{meta(start, ts), meta(start+1, ts), meta(start+2, ts)},
			want:   start + 2,
		},
		{
			name:   "mid-window gap truncates at the gap",
			blocks: []windowBlockMeta{meta(start, ts), meta(start+1, ts), meta(start+3, ts)},
			want:   start + 1,
		},
		{
			name:   "unsorted-looking gaps after the first break do not matter",
			blocks: []windowBlockMeta{meta(start, ts), meta(start+5, ts), meta(start+6, ts)},
			want:   start,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planWindow(start, tt.blocks)
			if got != tt.want {
				t.Fatalf("planWindow(start=%d) = %d, want %d", start, got, tt.want)
			}
		})
	}
}

func TestFirstOpsBlockWithoutHeader(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowTS := map[uint32]time.Time{101: ts, 102: ts}

	t.Run("all headers present", func(t *testing.T) {
		ops := []*model.Operation{op(101, 0, 0), op(102, 0, 0)}
		if bn, missing := firstOpsBlockWithoutHeader(ops, windowTS); missing {
			t.Fatalf("expected no missing header, got block %d", bn)
		}
	})

	t.Run("header missing returns the offending block", func(t *testing.T) {
		ops := []*model.Operation{op(101, 0, 0), op(103, 0, 0), op(104, 0, 0)}
		bn, missing := firstOpsBlockWithoutHeader(ops, windowTS)
		if !missing {
			t.Fatal("expected missing header for block 103")
		}
		if bn != 103 {
			t.Fatalf("expected block 103, got %d", bn)
		}
	})

	t.Run("no ops", func(t *testing.T) {
		if bn, missing := firstOpsBlockWithoutHeader(nil, windowTS); missing {
			t.Fatalf("expected no missing header for empty ops, got block %d", bn)
		}
	})
}

// TestProcessWindowHeadGapHoldsCursor covers F1 failure mode 1: the block
// header at the window start is missing while later blocks of the window
// exist. The window must not dispatch anything, must not query ops, and —
// most importantly — must not advance the cursor past the missing block.
func TestProcessWindowHeadGapHoldsCursor(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		// Block 101 (the window head) has no header; 102 and 103 do.
		blocks: []windowBlockMeta{meta(102, ts), meta(103, ts)},
		// Ops for 102 exist — the old code dispatched them with a zero
		// timestamp after advancing over 101.
		ops: []*model.Operation{op(102, 0, 0)},
	}
	cur := &fakeCursor{height: 100}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if oc.committed {
		t.Fatal("window with missing head block must not commit")
	}
	if oc.newHeight != 100 {
		t.Fatalf("newHeight = %d, want 100 (cursor held)", oc.newHeight)
	}
	if len(cur.advanced) != 0 {
		t.Fatalf("cursor advanced %v, want no advance", cur.advanced)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("dispatched %d ops, want 0", len(rec.calls))
	}
	if len(store.opsQueries) != 0 {
		t.Fatalf("WindowOps queried %v, want no ops query while the head is missing", store.opsQueries)
	}
}

// TestProcessWindowMidGapTruncates covers the normal gap semantics: a hole
// inside the window truncates processing at the gap; the cursor advances
// only to the last contiguous block, leaving the rest for the next window.
func TestProcessWindowMidGapTruncates(t *testing.T) {
	ts1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(3 * time.Second)
	store := &fakeWindowStore{
		// Headers for 101, 102 present; 103 missing; 104 present.
		blocks: []windowBlockMeta{meta(101, ts1), meta(102, ts2), meta(104, ts2)},
		ops: []*model.Operation{
			op(101, 0, 0), op(101, 0, 1),
			op(102, 0, 0),
			op(104, 0, 0), // belongs to a later window; must not be fetched
		},
	}
	cur := &fakeCursor{height: 100}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if !oc.committed {
		t.Fatal("window with mid gap must commit the contiguous prefix")
	}
	if oc.newHeight != 102 {
		t.Fatalf("newHeight = %d, want 102 (last contiguous block)", oc.newHeight)
	}
	if len(cur.advanced) != 1 || cur.advanced[0] != 102 {
		t.Fatalf("cursor advanced %v, want [102]", cur.advanced)
	}
	if len(store.opsQueries) != 1 || store.opsQueries[0] != [2]uint32{101, 102} {
		t.Fatalf("ops queries %v, want one query for [101,102]", store.opsQueries)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("dispatched %d ops, want 3", len(rec.calls))
	}
	for _, c := range rec.calls {
		if c.blockTS.IsZero() {
			t.Fatalf("op %s dispatched with zero block timestamp", c.op.ID)
		}
	}
	if rec.calls[0].blockTS != ts1 || rec.calls[2].blockTS != ts2 {
		t.Fatalf("per-block timestamps wrong: got %v, %v", rec.calls[0].blockTS, rec.calls[2].blockTS)
	}
}

// TestProcessWindowOpsWithoutHeaderHoldsCursor covers F1 failure mode 2:
// operations are visible for a block whose header document is not. With the
// head-gap hold in place this needs an inconsistent ops/header snapshot to
// reach (the defensive branch the pre-dispatch guard exists for — e.g. a
// header document dropped between the two window queries). The ops must not
// be dispatched with a zero timestamp and the cursor must hold.
func TestProcessWindowOpsWithoutHeaderHoldsCursor(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		// Headers contiguous for 101-102 …
		blocks: []windowBlockMeta{meta(101, ts), meta(102, ts)},
		// … but the ops snapshot also serves block 103 (outside the queried
		// range in a consistent store; the flag simulates the race).
		ops:            []*model.Operation{op(101, 0, 0), op(103, 0, 0)},
		opsIgnoreRange: true,
	}
	cur := &fakeCursor{height: 100}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if oc.committed {
		t.Fatal("window with ops-but-no-header must not commit")
	}
	if oc.newHeight != 100 {
		t.Fatalf("newHeight = %d, want 100 (cursor held)", oc.newHeight)
	}
	if len(cur.advanced) != 0 {
		t.Fatalf("cursor advanced %v, want no advance", cur.advanced)
	}
	// Nothing may be dispatched — not even the ops whose headers exist —
	// because the window replays as a whole once the missing header lands.
	if len(rec.calls) != 0 {
		t.Fatalf("dispatched %d ops, want 0 (no zero-timestamp dispatch)", len(rec.calls))
	}
}

// TestProcessWindowNormalCommits covers the healthy path: a fully present
// window dispatches every op in (block, trx, op) order with the correct
// per-block timestamp and advances the cursor to the effective end.
func TestProcessWindowNormalCommits(t *testing.T) {
	ts1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(3 * time.Second)
	ts3 := ts2.Add(3 * time.Second)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts1), meta(102, ts2), meta(103, ts3)},
		ops: []*model.Operation{
			op(101, 0, 0), op(101, 1, 0),
			op(102, 0, 0),
			op(103, 0, 0), op(103, 0, 1),
		},
	}
	cur := &fakeCursor{height: 100}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if !oc.committed {
		t.Fatal("fully present window must commit")
	}
	if oc.newHeight != 103 || oc.opsCount != 5 {
		t.Fatalf("newHeight=%d opsCount=%d, want 103 and 5", oc.newHeight, oc.opsCount)
	}
	if len(cur.advanced) != 1 || cur.advanced[0] != 103 {
		t.Fatalf("cursor advanced %v, want [103]", cur.advanced)
	}

	wantOrder := []string{
		"101:0:0", "101:1:0",
		"102:0:0",
		"103:0:0", "103:0:1",
	}
	if len(rec.calls) != len(wantOrder) {
		t.Fatalf("dispatched %d ops, want %d", len(rec.calls), len(wantOrder))
	}
	for i, want := range wantOrder {
		if rec.calls[i].op.ID != want {
			t.Fatalf("dispatch order[%d] = %s, want %s", i, rec.calls[i].op.ID, want)
		}
	}
	wantTS := map[uint32]time.Time{101: ts1, 102: ts2, 103: ts3}
	for _, c := range rec.calls {
		if wantTS[c.blockNum] != c.blockTS {
			t.Fatalf("op %s: blockTS = %v, want %v", c.op.ID, c.blockTS, wantTS[c.blockNum])
		}
	}
}

// TestProcessWindowEmptyWaits covers the caught-up case: nothing ingested in
// the window yet — no dispatch, no cursor movement, no ops query.
func TestProcessWindowEmptyWaits(t *testing.T) {
	store := &fakeWindowStore{}
	cur := &fakeCursor{height: 100}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if oc.committed {
		t.Fatal("empty window must not commit")
	}
	if oc.newHeight != 100 {
		t.Fatalf("newHeight = %d, want 100", oc.newHeight)
	}
	if len(cur.advanced) != 0 || len(store.opsQueries) != 0 || len(rec.calls) != 0 {
		t.Fatalf("empty window had side effects: advanced=%v opsQueries=%v dispatches=%d",
			cur.advanced, store.opsQueries, len(rec.calls))
	}
}

// TestProcessWindowFetchErrorHoldsCursor ensures query failures never move
// the cursor either — the window is retried unchanged.
func TestProcessWindowFetchErrorHoldsCursor(t *testing.T) {
	store := &fakeWindowStore{blocksErr: errSentinel}
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if oc.committed {
		t.Fatal("window with fetch error must not commit")
	}
	if oc.newHeight != 100 || len(cur.advanced) != 0 {
		t.Fatalf("fetch error must hold the cursor: newHeight=%d advanced=%v", oc.newHeight, cur.advanced)
	}
}

// TestProcessWindowCursorAdvanceErrorDoesNotReportCommit ensures a cursor
// write failure is not reported as committed: the next iteration replays the
// window (handler idempotency makes that safe).
func TestProcessWindowCursorAdvanceErrorDoesNotReportCommit(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0)},
	}
	cur := &fakeCursor{height: 100, advErr: errSentinel}
	rec := &dispatchRecorder{}
	d := NewDispatcher(&Context{})
	rec.register(d, "vote")

	p := newTestProcessor(store, cur, d)
	oc := p.processWindow(context.Background(), 100)

	if oc.committed {
		t.Fatal("cursor advance failure must not report the window as committed")
	}
	if oc.newHeight != 100 {
		t.Fatalf("newHeight = %d, want 100", oc.newHeight)
	}
	// Ops were dispatched before the failed commit — replay is the contract,
	// so the dispatch itself is expected here.
	if len(rec.calls) != 1 {
		t.Fatalf("dispatched %d ops, want 1 (replay handles the rest)", len(rec.calls))
	}
}
