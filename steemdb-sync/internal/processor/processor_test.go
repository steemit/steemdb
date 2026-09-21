package processor

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/processor/handlers"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// The tests below cover the window-planning and cursor-holding semantics of
// the processing loop (sync review findings F1, F5, N2):
//
//   - head gap: block header at the window start missing while later blocks
//     exist — the window must hold (no dispatch, no cursor advance);
//   - mid gap: gap inside the window — process up to the gap, then advance;
//   - ops present but header missing — hold instead of dispatching with a
//     zero timestamp;
//   - normal window — dispatch everything, advance to the contiguous end;
//   - handler error — hold the window (F5): the cursor must never advance
//     past an op whose handler failed; the window replays until it succeeds;
//   - poison op — after skip_error_ops_after_retries consecutive failed
//     attempts, skip the op loudly and let the window commit;
//   - mid-window flush failure — the inserter keeps the buffered models so
//     FlushAll / the next replay re-delivers them (N2).
//
// They run against fakes of the windowStore / cursorStore abstractions; the
// production mongoWindowStore and the live-Mongo write paths are not covered
// by any automated suite (test/e2e only exercises cold ingest, not the
// processor).

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

// newTestProcessor wires a Processor over fakes (no MongoDB — FlushAll is
// skipped for a nil inserter, matching production's guard). Tests that
// exercise the buffered-write path pass a real MongoInserter whose bulk-write
// primitive is replaced by a hook (see bulkRecorder below).
func newTestProcessor(store windowStore, cur cursorStore, dispatcher *Dispatcher) *Processor {
	return &Processor{
		cfg:        &config.Config{},
		dispatcher: dispatcher,
		cursor:     cur,
		inserter:   nil,
		store:      store,
		failures:   newFailedOpTracker(0),
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

// flakyHandler fails the first failCount calls and succeeds afterwards,
// counting every invocation.
type flakyHandler struct {
	failCount int
	calls     int
}

func (h *flakyHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	h.calls++
	if h.calls <= h.failCount {
		return errSentinel
	}
	return nil
}

// TestProcessWindowHandlerErrorHoldsCursor covers sync review finding F5:
// a handler error must hold the window — the cursor may not advance past an
// op whose derived writes just failed, because replay never returns for a
// block the cursor has passed. Once the transient error clears, the replayed
// window commits and the cursor advances.
func TestProcessWindowHandlerErrorHoldsCursor(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0), op(101, 0, 1)},
	}
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})
	flaky := &flakyHandler{failCount: 1}
	d.Register("vote", flaky) // op(101,0,0) is type "vote"
	ok := &flakyHandler{}
	d.Register("noop", ok)

	// Change the second op's type so only the first op fails.
	store.ops[1].OpType = "noop"

	p := newTestProcessor(store, cur, d)

	// Attempt 1: one op fails — dispatch continues past it (block semantics)
	// but the cursor must hold.
	oc := p.processWindow(context.Background(), 100)
	if oc.committed {
		t.Fatal("window with handler error must not commit")
	}
	if oc.newHeight != 100 || len(cur.advanced) != 0 {
		t.Fatalf("handler error must hold the cursor: newHeight=%d advanced=%v", oc.newHeight, cur.advanced)
	}
	if oc.errCount != 1 {
		t.Fatalf("errCount = %d, want 1", oc.errCount)
	}
	if flaky.calls != 1 || ok.calls != 1 {
		t.Fatalf("attempt 1 dispatch: flaky=%d ok=%d, want 1 and 1 (dispatch continues past the failure)", flaky.calls, ok.calls)
	}

	// Attempt 2: transient error cleared — the replay commits.
	oc = p.processWindow(context.Background(), 100)
	if !oc.committed {
		t.Fatal("window must commit once every handler succeeds")
	}
	if oc.newHeight != 101 || len(cur.advanced) != 1 || cur.advanced[0] != 101 {
		t.Fatalf("cursor advanced %v, want [101]", cur.advanced)
	}
	if oc.errCount != 0 || oc.skippedCount != 0 {
		t.Fatalf("errCount=%d skippedCount=%d, want 0 and 0", oc.errCount, oc.skippedCount)
	}
}

// procBulkRecorder captures the bulk-write batches issued through a test
// inserter (collection + the _id of every model), optionally failing the
// first failFirst calls.
type procBulkRecorder struct {
	calls     int
	failFirst int
	batches   []procBatch
}

// procBatch is one bulk-write attempt.
type procBatch struct {
	coll string
	ids  []string
	err  bool
}

// hook adapts the recorder to the inserter's bulk-write hook.
func (r *procBulkRecorder) hook(ctx context.Context, coll string, models []mongo.WriteModel) error {
	b := procBatch{coll: coll}
	for _, m := range models {
		if um, ok := m.(*mongo.UpdateOneModel); ok {
			if filter, ok := um.Filter.(bson.M); ok {
				if id, ok := filter["_id"].(string); ok {
					b.ids = append(b.ids, id)
				}
			}
		}
	}
	r.calls++
	if r.calls <= r.failFirst {
		b.err = true
		r.batches = append(r.batches, b)
		return errSentinel
	}
	r.batches = append(r.batches, b)
	return nil
}

// newInserterProcessor builds a Processor whose handlers write through a real
// MongoInserter backed by the recorder hook instead of MongoDB.
func newInserterProcessor(t *testing.T, store windowStore, cur cursorStore, d *Dispatcher, bufferLimit int) (*Processor, *handlers.MongoInserter, *procBulkRecorder) {
	t.Helper()
	rec := &procBulkRecorder{}
	ins := handlers.NewMongoInserter(nil)
	ins.SetBulkWriteHook(rec.hook)
	ins.BeginBatch(bufferLimit)
	t.Cleanup(ins.EndBatch)
	p := newTestProcessor(store, cur, d)
	p.inserter = ins
	return p, ins, rec
}

// TestProcessWindowMidFlushFailureRetainsBuffer covers sync review finding N2
// end to end: a mid-window buffer flush failure must not lose the buffered
// models. The failed batch is retained, re-delivered by the same attempt's
// FlushAll, the window is held by the error gate (F5), and the replay commits.
func TestProcessWindowMidFlushFailureRetainsBuffer(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0)},
	}
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})
	var ins *handlers.MongoInserter
	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		return ins.UpsertOne(ctx, "vote", op.ID, bson.M{"v": 1})
	}))

	// Buffer limit 1: the single append immediately triggers a mid-window
	// flush, which the recorder fails on the first call.
	p, ins, rec := newInserterProcessor(t, store, cur, d, 1)
	rec.failFirst = 1

	// Attempt 1: the mid-window flush fails → handler error → window held.
	// FlushAll still runs and must RE-DELIVER the retained model (N2).
	oc := p.processWindow(context.Background(), 100)
	if oc.committed || len(cur.advanced) != 0 {
		t.Fatalf("window must hold after mid-flush failure: committed=%v advanced=%v", oc.committed, cur.advanced)
	}
	if len(rec.batches) != 2 {
		t.Fatalf("bulk batches = %+v, want 2 (failed mid-flush + FlushAll retry)", rec.batches)
	}
	first, second := rec.batches[0], rec.batches[1]
	if !first.err {
		t.Fatalf("first batch %+v must be the failed mid-window flush", first)
	}
	if second.err || len(second.ids) != 1 || second.ids[0] != "101:0:0" {
		t.Fatalf("second batch %+v must be FlushAll re-delivering the retained model", second)
	}

	// Attempt 2: the flush now succeeds → the replay commits and advances.
	oc = p.processWindow(context.Background(), 100)
	if !oc.committed {
		t.Fatal("window must commit once the flush succeeds")
	}
	if len(cur.advanced) != 1 || cur.advanced[0] != 101 {
		t.Fatalf("cursor advanced %v, want [101]", cur.advanced)
	}
	if last := rec.batches[len(rec.batches)-1]; last.err || len(last.ids) != 1 || last.ids[0] != "101:0:0" {
		t.Fatalf("last batch = %+v, want successful idempotent re-write of the model", last)
	}
}

// TestProcessWindowHeldStillFlushesBuffers pins the N2×F5 interaction: a
// window held by handler errors still executes FlushAll, landing the
// successfully-dispatched buffered writes. Idempotent upserts make the later
// whole-window replay safe; the cursor stays put.
func TestProcessWindowHeldStillFlushesBuffers(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0), op(101, 0, 1)},
	}
	store.ops[1].OpType = "boom"
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})

	flaky := &flakyHandler{failCount: 1}
	var testInserterRef *handlers.MongoInserter
	d.Register("vote", OpHandlerFunc(func(ctx context.Context, op *model.Operation, blockTS time.Time) error {
		return testInserterRef.UpsertOne(ctx, "vote", op.ID, bson.M{"v": 1})
	}))
	d.Register("boom", flaky)

	p, testInserterRef, rec := newInserterProcessor(t, store, cur, d, 100)

	// Attempt 1: "boom" errors (window holds) but "vote" buffered a write —
	// FlushAll must land it anyway.
	oc := p.processWindow(context.Background(), 100)
	if oc.committed || len(cur.advanced) != 0 {
		t.Fatalf("window must hold: committed=%v advanced=%v", oc.committed, cur.advanced)
	}
	if len(rec.batches) != 1 || rec.batches[0].err || len(rec.batches[0].ids) != 1 || rec.batches[0].ids[0] != "101:0:0" {
		t.Fatalf("batches = %+v, want FlushAll to land the successfully-dispatched write [101:0:0]", rec.batches)
	}

	// Attempt 2: everything succeeds — the replay re-writes the same model
	// (idempotent upsert) and commits.
	oc = p.processWindow(context.Background(), 100)
	if !oc.committed || len(cur.advanced) != 1 || cur.advanced[0] != 101 {
		t.Fatalf("window must commit on replay: committed=%v advanced=%v", oc.committed, cur.advanced)
	}
}

// captureLogs swaps the standard logger's output for a buffer and restores it.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldFlags := log.Flags()
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
	})
	return &buf
}

// TestProcessWindowPoisonOpSkippedAfterRetries covers the escape hatch: with
// skip_error_ops_after_retries=2, an op that fails two consecutive window
// attempts is skipped (loudly, never dispatched again) and the window commits
// past it. A sibling op that keeps succeeding is never sacrificed.
func TestProcessWindowPoisonOpSkippedAfterRetries(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0), op(101, 0, 1)},
	}
	store.ops[1].OpType = "healthy"
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})
	poison := &flakyHandler{failCount: 1 << 30} // always fails
	healthy := &flakyHandler{}
	d.Register("vote", poison) // op(101,0,0)
	d.Register("healthy", healthy)

	logs := captureLogs(t)

	p := newTestProcessor(store, cur, d)
	p.failures = newFailedOpTracker(2)

	// Attempts 1 and 2: the poison op fails, the window holds; both ops are
	// dispatched each time.
	for i := 1; i <= 2; i++ {
		oc := p.processWindow(context.Background(), 100)
		if oc.committed {
			t.Fatalf("attempt %d: window must hold while the poison op fails", i)
		}
		if poison.calls != i || healthy.calls != i {
			t.Fatalf("attempt %d: poison=%d healthy=%d calls, want %d and %d", i, poison.calls, healthy.calls, i, i)
		}
	}

	// Attempt 3: the poison op exhausted its retries — skipped, not
	// dispatched — and the window commits past it.
	oc := p.processWindow(context.Background(), 100)
	if !oc.committed {
		t.Fatal("window must commit once the poison op is skipped")
	}
	if poison.calls != 2 {
		t.Fatalf("poison op dispatched %d times, want 2 (skipped on attempt 3)", poison.calls)
	}
	if healthy.calls != 3 {
		t.Fatalf("healthy op dispatched %d times, want 3", healthy.calls)
	}
	if len(cur.advanced) != 1 || cur.advanced[0] != 101 {
		t.Fatalf("cursor advanced %v, want [101]", cur.advanced)
	}
	if oc.skippedCount != 1 {
		t.Fatalf("skippedCount = %d, want 1", oc.skippedCount)
	}
	if !strings.Contains(logs.String(), "SKIP op 101:0:0") ||
		!strings.Contains(logs.String(), "skip_error_ops_after_retries=2") {
		t.Fatalf("skip log must name the op and the config; got:\n%s", logs.String())
	}
}

// TestProcessWindowPoisonOpNeverSkippedByDefault pins the safe default: with
// skip_error_ops_after_retries=0 (default) a persistently failing op is
// re-dispatched on every attempt and the window never commits past it —
// a stalled cursor is visible, silent data loss is not.
func TestProcessWindowPoisonOpNeverSkippedByDefault(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0)},
	}
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})
	poison := &flakyHandler{failCount: 1 << 30}
	d.Register("vote", poison)

	logs := captureLogs(t)
	p := newTestProcessor(store, cur, d) // default tracker: threshold 0

	for i := 1; i <= 3; i++ {
		oc := p.processWindow(context.Background(), 100)
		if oc.committed {
			t.Fatalf("attempt %d: window must never commit past a failing op by default", i)
		}
	}
	if poison.calls != 3 {
		t.Fatalf("poison op dispatched %d times, want 3 (retried every attempt)", poison.calls)
	}
	if len(cur.advanced) != 0 {
		t.Fatalf("cursor advanced %v, want none", cur.advanced)
	}
	if strings.Contains(logs.String(), "SKIP op") {
		t.Fatalf("default configuration must never skip; got:\n%s", logs.String())
	}
}

// TestProcessWindowRecoveredOpNotSkipped verifies the tracker's "consecutive"
// semantics through the window loop: an op that fails once and then succeeds
// is not skipped even with the escape hatch armed, and the window commits on
// the recovering attempt.
func TestProcessWindowRecoveredOpNotSkipped(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeWindowStore{
		blocks: []windowBlockMeta{meta(101, ts)},
		ops:    []*model.Operation{op(101, 0, 0)},
	}
	cur := &fakeCursor{height: 100}
	d := NewDispatcher(&Context{})
	flaky := &flakyHandler{failCount: 1} // fails once, then recovers
	d.Register("vote", flaky)

	logs := captureLogs(t)
	p := newTestProcessor(store, cur, d)
	p.failures = newFailedOpTracker(2)

	if oc := p.processWindow(context.Background(), 100); oc.committed {
		t.Fatal("first attempt must hold (transient failure)")
	}
	oc := p.processWindow(context.Background(), 100)
	if !oc.committed || len(cur.advanced) != 1 || cur.advanced[0] != 101 {
		t.Fatalf("window must commit on recovery: committed=%v advanced=%v", oc.committed, cur.advanced)
	}
	if oc.skippedCount != 0 || strings.Contains(logs.String(), "SKIP op") {
		t.Fatalf("a recovered op must never be skipped; skippedCount=%d logs:\n%s", oc.skippedCount, logs.String())
	}
	if flaky.calls != 2 {
		t.Fatalf("flaky op dispatched %d times, want 2", flaky.calls)
	}
}
