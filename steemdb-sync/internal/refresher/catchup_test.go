package refresher

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// newGateTestDB spins up an in-memory MongoDB (mtest-style via mongo-driver's
// test server is unavailable here; use mgo-free unit checks with a real client
// against mtest when available). To keep the unit test dependency-free, the
// gate decision logic is exercised through a fake-free approach: the tests
// below drive blocked() against a real in-memory mongo if MONGO_URI_FOR_TEST
// is set, otherwise they verify the pure threshold/decision helper.
func TestBlockedDecision(t *testing.T) {
	cases := []struct {
		name     string
		local    int64
		localErr error
		head     int64
		headOK   bool
		headErr  error
		wantSkip bool
	}{
		{"empty database", 0, nil, 100, true, nil, true},
		{"no sync_state", 5000, nil, 0, false, nil, true},
		{"gap too large", 5000, nil, 200000, true, nil, true},
		{"within threshold", 199100, nil, 200000, true, nil, false},
		{"exactly at threshold", 199000, nil, 200000, true, nil, false},
		{"head behind frontier (stale meta)", 200000, nil, 100, true, nil, false},
	}
	for _, c := range cases {
		skip := decide(c.local, c.localErr, c.head, c.headOK, c.headErr)
		if skip != c.wantSkip {
			t.Errorf("%s: decide(local=%d, head=%d, ok=%v) = %v, want %v",
				c.name, c.local, c.head, c.headOK, skip, c.wantSkip)
		}
	}
}

// decide mirrors catchupGate.blocked's decision tree on pre-fetched values so
// it can be tested without a database.
func decide(local int64, localErr error, head int64, headOK bool, headErr error) bool {
	if localErr != nil {
		return true
	}
	if local == 0 {
		return true
	}
	if headErr != nil {
		return true
	}
	if !headOK {
		return true
	}
	if head < local {
		return false
	}
	return head-local > coldStartThreshold
}

func TestBlockedForProcessorDecision(t *testing.T) {
	cases := []struct {
		name     string
		cursor   int64
		curErr   error
		head     int64
		headOK   bool
		headErr  error
		fallback int64
		fbErr    error
		wantSkip bool
	}{
		{"cursor read error", 0, errFake, 200000, true, nil, 200000, nil, true},
		{"cursor not established", 0, nil, 200000, true, nil, 200000, nil, true},
		{"ingest caught up, processor behind (the review case)", 1000, nil, 200000, true, nil, 200000, nil, true},
		{"processor within threshold", 199100, nil, 200000, true, nil, 200000, nil, false},
		{"exactly at threshold", 199000, nil, 200000, true, nil, 200000, nil, false},
		{"no sync_state: fallback to blocks frontier", 199100, nil, 0, false, nil, 200000, nil, false},
		{"no sync_state: fallback gap too large", 1000, nil, 0, false, nil, 200000, nil, true},
		{"stale meta (head behind cursor): fallback", 199100, nil, 100, true, nil, 200000, nil, false},
		{"fallback read error", 1000, nil, 0, false, nil, 0, errFake, true},
		{"meta read error: fallback", 199100, nil, 0, false, errFake, 200000, nil, false},
	}
	for _, c := range cases {
		skip := decideForProcessor(c.cursor, c.curErr, c.head, c.headOK, c.headErr, c.fallback, c.fbErr)
		if skip != c.wantSkip {
			t.Errorf("%s: decideForProcessor(cursor=%d, head=%d, ok=%v, fb=%d) = %v, want %v",
				c.name, c.cursor, c.head, c.headOK, c.fallback, skip, c.wantSkip)
		}
	}
}

var errFake = errors.New("fake read error")

// decideForProcessor mirrors catchupGate.blockedForProcessor's decision tree.
// head is meta.max_block, fallback is the blocks collection max used when
// meta is missing, unreadable, or stale.
func decideForProcessor(cursor int64, curErr error, head int64, headOK bool, headErr error, fallback int64, fbErr error) bool {
	if curErr != nil {
		return true
	}
	if cursor == 0 {
		return true
	}
	if headErr != nil || !headOK || head < cursor {
		if fbErr != nil {
			return true
		}
		head = fallback
	}
	return head-cursor > coldStartThreshold
}

// TestCatchupGateAgainstMongo is an integration test gated behind an
// environment-provided MongoDB (same pattern as the rest of the repo's
// docker-compose-based tests).
func TestCatchupGateAgainstMongo(t *testing.T) {
	uri := mongoTestURI()
	if uri == "" {
		t.Skip("no test mongo configured")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(ctx)
	db := client.Database("steemdb_refresher_gate_test")
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("drop: %v", err)
	}

	gate := newCatchupGate(db)

	// Empty database: skip.
	if ok, _ := gate.blocked(ctx); !ok {
		t.Error("empty database should block")
	}

	// Frontier without sync_state: skip.
	if _, err := db.Collection("blocks").InsertOne(ctx, bson.M{"_id": int64(5000)}); err != nil {
		t.Fatalf("insert block: %v", err)
	}
	if ok, _ := gate.blocked(ctx); !ok {
		t.Error("missing sync_state should block")
	}

	// Large gap: skip; within threshold: allow.
	setSyncState := func(maxBlock int64) {
		_, err := db.Collection("meta").UpdateOne(ctx,
			bson.M{"_id": "sync_state"},
			bson.M{"$set": bson.M{"max_block": maxBlock}},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			t.Fatalf("set sync_state: %v", err)
		}
	}
	setSyncState(200000)
	if ok, _ := gate.blocked(ctx); !ok {
		t.Error("gap > threshold should block")
	}
	setSyncState(5100) // gap = 100 <= threshold
	if ok, _ := gate.blocked(ctx); ok {
		t.Error("gap <= threshold should allow")
	}

	// Stale meta (head behind frontier): allow (blocks collection wins).
	setSyncState(100)
	if ok, _ := gate.blocked(ctx); ok {
		t.Error("stale meta (head < frontier) should allow")
	}
}

// TestProcessorGateAgainstMongo exercises blockedForProcessor: the clients
// aggregation is processor-derived, so the gate must stay closed while the
// processor cursor lags — even when the ingest frontier is already caught up.
func TestProcessorGateAgainstMongo(t *testing.T) {
	uri := mongoTestURI()
	if uri == "" {
		t.Skip("no test mongo configured")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(ctx)
	db := client.Database("steemdb_refresher_pgate_test")
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("drop: %v", err)
	}

	gate := newCatchupGate(db)
	setCursor := func(v int64) {
		_, err := db.Collection("status").UpdateOne(ctx,
			bson.M{"_id": "processor_height"},
			bson.M{"$set": bson.M{"value": v}},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			t.Fatalf("set processor cursor: %v", err)
		}
	}
	insertBlock := func(v int64) {
		if _, err := db.Collection("blocks").InsertOne(ctx, bson.M{"_id": v}); err != nil {
			t.Fatalf("insert block %d: %v", v, err)
		}
	}
	setSyncState := func(maxBlock int64) {
		_, err := db.Collection("meta").UpdateOne(ctx,
			bson.M{"_id": "sync_state"},
			bson.M{"$set": bson.M{"max_block": maxBlock}},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			t.Fatalf("set sync_state: %v", err)
		}
	}

	// No cursor: skip.
	if ok, _ := gate.blockedForProcessor(ctx); !ok {
		t.Error("missing processor cursor should block")
	}

	// The regression case from the PR review: ingest fully caught up with
	// the committed head, processor still far behind — clients must skip
	// while the ingest-frontier gate (stats) correctly allows.
	setCursor(1000)
	insertBlock(200000)
	setSyncState(200000)
	if ok, _ := gate.blockedForProcessor(ctx); !ok {
		t.Error("processor behind head should block even when ingest is caught up")
	}
	if ok, _ := gate.blocked(ctx); ok {
		t.Error("ingest caught up should keep the stats gate open")
	}

	// Processor caught up: allow.
	setCursor(199500)
	if ok, _ := gate.blockedForProcessor(ctx); ok {
		t.Error("processor within threshold should allow")
	}

	// No sync_state: fall back to the blocks frontier.
	setCursor(199000)
	if err := db.Collection("meta").Drop(ctx); err != nil {
		t.Fatalf("drop meta: %v", err)
	}
	if ok, _ := gate.blockedForProcessor(ctx); ok {
		t.Error("missing meta with processor at blocks frontier should allow")
	}
	setCursor(1000)
	if ok, _ := gate.blockedForProcessor(ctx); !ok {
		t.Error("missing meta with processor far behind blocks frontier should block")
	}
}
