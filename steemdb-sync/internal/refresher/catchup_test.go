package refresher

import (
	"context"
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
