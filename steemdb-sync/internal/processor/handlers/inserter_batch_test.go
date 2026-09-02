package handlers

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestCanonicalFilterKey(t *testing.T) {
	// Same content, different map iteration order → same key.
	a := bson.M{"_block": 123, "voter": "alice"}
	b := bson.M{"voter": "alice", "_block": 123}
	if canonicalFilterKey(a) != canonicalFilterKey(b) {
		t.Errorf("canonicalFilterKey must be deterministic: %q != %q",
			canonicalFilterKey(a), canonicalFilterKey(b))
	}

	// Different content → different key.
	c := bson.M{"_block": 124, "voter": "alice"}
	if canonicalFilterKey(a) == canonicalFilterKey(c) {
		t.Errorf("different filters must produce different keys")
	}
}

func TestBatchModeState(t *testing.T) {
	m := NewMongoInserter(nil)

	m.BeginBatch(10)
	if !m.batchMode {
		t.Fatal("BeginBatch must enable batch mode")
	}
	if m.bufferLimit != 10 {
		t.Fatalf("bufferLimit = %d, want 10", m.bufferLimit)
	}

	// Batched QueueAccountDirty coalesces without touching the database.
	if err := m.QueueAccountDirty(nil, "alice"); err != nil {
		t.Fatalf("QueueAccountDirty in batch mode: %v", err)
	}
	if err := m.QueueAccountDirty(nil, "alice"); err != nil {
		t.Fatalf("QueueAccountDirty repeat: %v", err)
	}
	if len(m.dirtyAccounts) != 1 {
		t.Fatalf("dirtyAccounts = %d entries, want 1 (coalesced)", len(m.dirtyAccounts))
	}

	m.EndBatch()
	if m.batchMode {
		t.Fatal("EndBatch must disable batch mode")
	}
	if m.dirtyAccounts != nil {
		t.Fatal("EndBatch must discard buffers")
	}
}
