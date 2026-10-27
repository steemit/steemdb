package refresher

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestClientsTickerAgainstMongo runs the clients refresher tick against a real
// MongoDB and pins the three defects the 2026-10 audit found in it:
//
//  1. the app regex required a "name/version" shape, so 13% of the posts that do
//     name a client (no version suffix) were silently dropped;
//  2. reward was summed with `$add`, which returns null as soon as one payout
//     field is missing, so whole days read reward 0 while the post counts were
//     right;
//  3. the snapshot carried no refresh timestamp and no coverage counters, so a
//     stale snapshot and an unattributed share of posts were both invisible.
//
// It also pins a fourth defect found in review: `json_metadata.app` written as
// an array passed the `$type: "string"` match and made `$split` abort, so a
// single such document stopped the tick before the snapshot was written. The
// array fixtures below make this test fail at "snapshot not written" if the
// type guard regresses.
func TestClientsTickerAgainstMongo(t *testing.T) {
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

	db := client.Database("steemdb_refresher_clients_test")
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("drop: %v", err)
	}

	// Open the catch-up gate: processor cursor at 1000 with the blocks
	// frontier at the same height.
	if _, err := db.Collection("status").InsertOne(ctx, bson.M{"_id": "processor_height", "value": int64(1000)}); err != nil {
		t.Fatalf("seed processor_height: %v", err)
	}
	if _, err := db.Collection("blocks").InsertOne(ctx, bson.M{"_id": int64(1000)}); err != nil {
		t.Fatalf("seed block: %v", err)
	}

	created := time.Now().UTC().Add(-2 * time.Hour)
	comments := []bson.M{
		// Conventional name/version: counted, reward 10 SBD.
		{"_id": "a/p1", "created": created, "json_metadata": bson.M{"app": "steemit/0.1.0"},
			"total_payout_value": 8.0, "pending_payout_value": 1.0, "total_pending_payout_value": 1.0},
		// No version suffix — the case the old regex dropped. Two payout legs
		// missing: `$add` would have yielded null and zeroed the day, while
		// `$sum` ignores them (so the assertion below discriminates the
		// historical `$add` defect only through the total, not through the
		// guards).
		{"_id": "a/p2", "created": created, "json_metadata": bson.M{"app": "dkscore-poststudio"},
			"pending_payout_value": 5.0},
		// Empty app string: scanned, not attributed.
		{"_id": "a/p3", "created": created, "json_metadata": bson.M{"app": ""}},
		// No app field at all: scanned, not attributed.
		{"_id": "a/p4", "created": created, "json_metadata": bson.M{}},
		// App with a leading slash leaves an empty client name: skipped.
		{"_id": "a/p5", "created": created, "json_metadata": bson.M{"app": "/1.2.3"}},
		// app as an array of strings: the query-level `$type: "string"` match
		// does NOT exclude it (MongoDB matches an array when an element is a
		// string), and the unguarded `$split` then aborted the whole tick —
		// no snapshot written at all. Both of these are scanned, neither is
		// attributed.
		{"_id": "a/p7", "created": created, "json_metadata": bson.M{"app": []interface{}{"steemx/1.0"}}},
		{"_id": "a/p8", "created": created, "json_metadata": bson.M{"app": []interface{}{"a", "b"}}},
		// Outside the 90-day window: ignored entirely.
		{"_id": "a/p6", "created": created.AddDate(0, 0, -120), "json_metadata": bson.M{"app": "old/1.0"}},
	}
	for _, doc := range comments {
		if _, err := db.Collection("comment").InsertOne(ctx, doc); err != nil {
			t.Fatalf("seed comment %v: %v", doc["_id"], err)
		}
	}

	c := newClientsRefresher(db)
	c.tick(ctx)

	var snapshot struct {
		UpdatedAt    *time.Time `bson:"updated_at"`
		WindowFrom   *time.Time `bson:"window_from"`
		WindowTo     *time.Time `bson:"window_to"`
		PostsScanned int        `bson:"posts_scanned"`
		PostsWithApp int        `bson:"posts_with_app"`
		Data         []struct {
			ID struct {
				Year  int `bson:"year"`
				Month int `bson:"month"`
				Day   int `bson:"day"`
			} `bson:"_id"`
			Clients []struct {
				Client string  `bson:"client"`
				Count  int     `bson:"count"`
				Reward float64 `bson:"reward"`
			} `bson:"clients"`
			Reward float64 `bson:"reward"`
			Total  int     `bson:"total"`
		} `bson:"data"`
	}
	if err := db.Collection("status").FindOne(ctx, bson.M{"_id": "clients-snapshot"}).Decode(&snapshot); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}

	if snapshot.UpdatedAt == nil || snapshot.WindowFrom == nil || snapshot.WindowTo == nil {
		t.Errorf("snapshot metadata missing: updated_at=%v window_from=%v window_to=%v",
			snapshot.UpdatedAt, snapshot.WindowFrom, snapshot.WindowTo)
	}
	// 7 comments inside the 90-day window. Only 2 of them are attributed to a
	// client (steemit/0.1.0 and dkscore-poststudio): the "/1.2.3" app leaves an
	// empty client name and is dropped by the aggregation, and the two array
	// apps are dropped by the same match, so none of them may be counted as
	// attributed — counting them made the coverage claim a post the client
	// lists did not contain.
	if snapshot.PostsScanned != 7 || snapshot.PostsWithApp != 2 {
		t.Errorf("coverage = %d/%d, want 2/7 (the out-of-window comment is not scanned; \"/1.2.3\" and the array apps are scanned but unattributed)",
			snapshot.PostsWithApp, snapshot.PostsScanned)
	}

	if len(snapshot.Data) != 1 {
		t.Fatalf("snapshot has %d days, want 1: %+v", len(snapshot.Data), snapshot.Data)
	}
	day := snapshot.Data[0]
	if day.Total != 2 || day.Reward != 15.0 {
		t.Errorf("day totals = %d posts / %v SBD, want 2/15 (10 + 5)", day.Total, day.Reward)
	}

	byClient := map[string]float64{}
	counts := map[string]int{}
	for _, entry := range day.Clients {
		byClient[entry.Client] = entry.Reward
		counts[entry.Client] = entry.Count
	}
	if counts["steemit"] != 1 || byClient["steemit"] != 10.0 {
		t.Errorf("steemit = %d posts / %v SBD, want 1/10", counts["steemit"], byClient["steemit"])
	}
	if counts["dkscore-poststudio"] != 1 {
		t.Errorf("dkscore-poststudio missing from the snapshot; a client name does not need a /version suffix")
	}
	// The post has only pending_payout_value set; `$add` over a missing
	// total_payout_value would have produced null and a 0 reward.
	if byClient["dkscore-poststudio"] != 5.0 {
		t.Errorf("dkscore-poststudio reward = %v SBD, want 5 (missing payout fields are summed as 0, not null)",
			byClient["dkscore-poststudio"])
	}
	if _, ok := byClient[""]; ok {
		t.Error("empty client name reached the snapshot")
	}
	if _, ok := byClient["old"]; ok {
		t.Error("a post older than the 90-day window reached the snapshot")
	}
}
