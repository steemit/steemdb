package handlers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// bulkRecorder replaces the driver bulk write in tests: it records every
// batch (collection + the `_id` of every model) and consults a per-collection
// failure policy. The policy receives the collection name and the number of
// previous calls for that collection, and returns whether this call fails.
type bulkRecorder struct {
	mu      sync.Mutex
	batches []recordedBatch
	fail    func(coll string, call int) bool
}

// recordedBatch is one bulk-write attempt.
type recordedBatch struct {
	coll string
	ids  []interface{}
	err  bool
}

func (r *bulkRecorder) hook(ctx context.Context, coll string, models []mongo.WriteModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := 0
	for _, b := range r.batches {
		if b.coll == coll {
			call++
		}
	}
	b := recordedBatch{coll: coll}
	for _, m := range models {
		if um, ok := m.(*mongo.UpdateOneModel); ok {
			if filter, ok := um.Filter.(bson.M); ok {
				b.ids = append(b.ids, filter["_id"])
			}
		}
	}
	if r.fail != nil && r.fail(coll, call) {
		b.err = true
		r.batches = append(r.batches, b)
		return errors.New("simulated bulk write failure")
	}
	r.batches = append(r.batches, b)
	return nil
}

func (r *bulkRecorder) snapshot() []recordedBatch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedBatch(nil), r.batches...)
}

// TestFlushFailureKeepsModels covers sync review finding N2 at the inserter
// level: a mid-window flush (here triggered by the buffer limit) that fails
// must KEEP the buffered models so a later flush retries them. Before the fix
// the bucket was cleared before the error check and the failed batch was
// permanently lost once the window committed over the remaining buffers.
func TestFlushFailureKeepsModels(t *testing.T) {
	rec := &bulkRecorder{fail: func(coll string, call int) bool { return call == 0 }}
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(rec.hook)
	m.BeginBatch(1) // every append immediately hits the buffer limit
	defer m.EndBatch()

	// First append triggers a buffer-limit flush, which the recorder fails.
	if err := m.UpsertOne(context.Background(), "vote", "doc1", bson.M{"v": 1}); err == nil {
		t.Fatal("buffer-limit flush failure must surface as an error")
	}
	if got := len(m.buckets["vote"].models); got != 1 {
		t.Fatalf("bucket after failed flush holds %d models, want 1 (models must be retained)", got)
	}

	// The retry (FlushAll) must re-deliver the retained model.
	if err := m.FlushAll(context.Background()); err != nil {
		t.Fatalf("FlushAll retry failed: %v", err)
	}
	if got := len(m.buckets["vote"].models); got != 0 {
		t.Fatalf("bucket after successful FlushAll holds %d models, want 0", got)
	}
	batches := rec.snapshot()
	if len(batches) != 2 {
		t.Fatalf("recorded %d batches, want 2 (failed flush + FlushAll retry)", len(batches))
	}
	if !batches[0].err || !idsEqual(batches[0].ids, "doc1") {
		t.Fatalf("first batch = %+v, want failed batch with [doc1]", batches[0])
	}
	if batches[1].err || !idsEqual(batches[1].ids, "doc1") {
		t.Fatalf("second batch = %+v, want successful retry re-delivering [doc1] (N2 retention)", batches[1])
	}
}

// TestFlushFailureErrorIncludesModelCount guards the error message: the old
// code cleared the bucket before formatting the message, so it always
// reported "(0 models)".
func TestFlushFailureErrorIncludesModelCount(t *testing.T) {
	rec := &bulkRecorder{fail: func(coll string, call int) bool { return true }}
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(rec.hook)
	m.BeginBatch(3) // two models buffer without hitting the limit
	defer m.EndBatch()

	if err := m.UpsertOne(context.Background(), "vote", "a", bson.M{"v": 1}); err != nil {
		t.Fatalf("first append must buffer: %v", err)
	}
	if err := m.UpsertOne(context.Background(), "vote", "b", bson.M{"v": 2}); err != nil {
		t.Fatalf("second append must buffer: %v", err)
	}
	err := m.FlushAll(context.Background())
	if err == nil {
		t.Fatal("FlushAll must fail while the recorder fails")
	}
	if !strings.Contains(err.Error(), "(2 models)") {
		t.Fatalf("error %q must report the retained model count", err)
	}
}

// TestCollisionFlushFailureKeepsBufferAndOrder pins the same-document
// collision path: when the ordering flush fails, the NEW model must not be
// appended (the buffer would then hold two writes to one document in a single
// unordered bulk), the earlier write stays buffered, and the next collision
// re-attempts the flush — so the later op still lands after the earlier one.
func TestCollisionFlushFailureKeepsBufferAndOrder(t *testing.T) {
	rec := &bulkRecorder{fail: func(coll string, call int) bool { return call == 0 }}
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(rec.hook)
	m.BeginBatch(100)
	defer m.EndBatch()

	ctx := context.Background()
	if err := m.UpsertOne(ctx, "vote", "doc1", bson.M{"v": 1}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	// Same _id again: same-filter collision → ordering flush → fails.
	if err := m.UpsertOne(ctx, "vote", "doc1", bson.M{"v": 2}); err == nil {
		t.Fatal("collision flush failure must surface as an error")
	}
	b := m.buckets["vote"]
	if got := len(b.models); got != 1 {
		t.Fatalf("bucket holds %d models after failed collision flush, want 1 (new model must not be appended)", got)
	}
	if _, still := b.keys[`{"_id":"doc1"}`]; !still {
		t.Fatalf("collision key must stay registered so the retry re-flushes: keys=%v", b.keys)
	}

	// Retry the same write: the collision fires again, the flush now
	// succeeds (landing the EARLIER write), and the new model buffers.
	if err := m.UpsertOne(ctx, "vote", "doc1", bson.M{"v": 2}); err != nil {
		t.Fatalf("append after successful collision flush: %v", err)
	}
	batches := rec.snapshot()
	if len(batches) != 2 || !batches[0].err || batches[1].err || !idsEqual(batches[1].ids, "doc1") {
		t.Fatalf("batches = %+v, want failed attempt then a successful retry landing the retained earlier write [doc1]", batches)
	}
	if got := len(b.models); got != 1 {
		t.Fatalf("bucket holds %d models after retried collision, want 1 (the new write)", got)
	}
	// FlushAll lands the later write after the earlier one — later op wins.
	if err := m.FlushAll(ctx); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}
	if batches = rec.snapshot(); len(batches) != 3 || batches[2].err || !idsEqual(batches[2].ids, "doc1") {
		t.Fatalf("batches = %+v, want the later write flushed last", batches)
	}
}

// TestFlushAllPartialFailureRetriesOnlyFailedBucket covers the partial-success
// semantics of a multi-collection FlushAll: the bucket that landed is cleared
// (its writes must not be re-delivered), the bucket that failed keeps every
// model, and the next FlushAll retries only the failed one.
func TestFlushAllPartialFailureRetriesOnlyFailedBucket(t *testing.T) {
	rec := &bulkRecorder{fail: func(coll string, call int) bool {
		return coll == "vote" && call == 0
	}}
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(rec.hook)
	m.BeginBatch(100)
	defer m.EndBatch()

	ctx := context.Background()
	if err := m.UpsertOne(ctx, "vote", "v1", bson.M{"v": 1}); err != nil {
		t.Fatalf("append vote: %v", err)
	}
	if err := m.UpsertOne(ctx, "transfer", "t1", bson.M{"v": 1}); err != nil {
		t.Fatalf("append transfer: %v", err)
	}

	if err := m.FlushAll(ctx); err == nil {
		t.Fatal("FlushAll must fail while the vote bucket fails")
	}
	if got := len(m.buckets["vote"].models); got != 1 {
		t.Fatalf("failed vote bucket holds %d models, want 1", got)
	}
	if got := len(m.buckets["transfer"].models); got != 0 {
		t.Fatalf("successful transfer bucket holds %d models, want 0 (already landed)", got)
	}

	if err := m.FlushAll(ctx); err != nil {
		t.Fatalf("FlushAll retry: %v", err)
	}
	batches := rec.snapshot()
	if len(batches) != 3 {
		t.Fatalf("recorded %d batches, want 3", len(batches))
	}
	// Batch order is map-iteration dependent for the first FlushAll; assert
	// by content: one transfer batch (landed, not retried), two vote batches
	// (failed + retried).
	var voteErr, voteOK, transferOK bool
	for _, b := range batches {
		switch {
		case b.coll == "vote" && b.err:
			voteErr = true
		case b.coll == "vote" && !b.err:
			voteOK = true
		case b.coll == "transfer" && !b.err:
			transferOK = true
		}
	}
	if !voteErr || !voteOK || !transferOK {
		t.Fatalf("batches = %+v, want failed vote + retried vote + single transfer", batches)
	}
}

// TestDirtyMarksRetainedOnFlushFailure mirrors N2 for the coalesced account
// dirty marks: a failed flush keeps them so the next flush retries.
func TestDirtyMarksRetainedOnFlushFailure(t *testing.T) {
	rec := &bulkRecorder{fail: func(coll string, call int) bool { return call == 0 }}
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(rec.hook)
	m.BeginBatch(100)
	defer m.EndBatch()

	if err := m.QueueAccountDirty(context.Background(), "alice"); err != nil {
		t.Fatalf("QueueAccountDirty: %v", err)
	}
	if err := m.QueueAccountDirty(context.Background(), "bob"); err != nil {
		t.Fatalf("QueueAccountDirty: %v", err)
	}
	if err := m.FlushAll(context.Background()); err == nil {
		t.Fatal("FlushAll must fail while the account write fails")
	}
	if got := len(m.dirtyAccounts); got != 2 {
		t.Fatalf("dirtyAccounts after failed flush = %d, want 2 (marks retained)", got)
	}
	if err := m.FlushAll(context.Background()); err != nil {
		t.Fatalf("FlushAll retry: %v", err)
	}
	if got := len(m.dirtyAccounts); got != 0 {
		t.Fatalf("dirtyAccounts after successful flush = %d, want 0", got)
	}
}

func idsEqual(got []interface{}, want ...interface{}) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
