package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// labsTestService returns a LabsService bound to the integration-test MongoDB
// (STEEMDB_TEST_MONGO_URI, same convention as steemdb-sync's refresher tests),
// or skips the test when no database is configured.
func labsTestService(t *testing.T) (*LabsService, context.Context) {
	t.Helper()

	uri := os.Getenv("STEEMDB_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("no test mongo configured (STEEMDB_TEST_MONGO_URI)")
	}

	logger, err := utils.NewLogger(utils.LogConfig{Level: "error", Format: "text"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	db, err := database.NewMongoDB(utils.MongoDBConfig{
		URI:      uri,
		Database: "steemdb_web_labs_test",
		PoolSize: 5,
		Timeout:  10 * time.Second,
	}, logger)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ctx := context.Background()
	if err := db.Database().Drop(ctx); err != nil {
		t.Fatalf("drop: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Database().Drop(context.Background())
		_ = db.Close(context.Background())
	})

	return NewLabsService(db, nil, logger), ctx
}

// TestBenefactorsAgainstMongo runs the real aggregation against MongoDB and
// pins the three defects the 2026-10 audit found: the unbounded $group (code
// 292), the ascending sort that returned the oldest 10 days, and the
// per-day benefactor array not being ordered by reward.
func TestBenefactorsAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	collection := svc.db.Collection("benefactor_reward")
	// The production index (steemdb-sync's inventory) leads with _ts; the
	// window $match must be servable by it.
	if _, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "_ts", Value: 1}}}); err != nil {
		t.Fatalf("create index: %v", err)
	}

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)

	// 12 recent UTC days (more than the 10 the endpoint returns), two
	// benefactors each: "big" earns more than "small" so the array order is
	// observable.
	for d := 0; d < 12; d++ {
		day := today.AddDate(0, 0, -d).Add(6 * time.Hour)
		for i, doc := range []bson.M{
			{"benefactor": "big", "reward": 1000.0 + float64(d)},
			{"benefactor": "small", "reward": 1.0 + float64(d)},
			{"benefactor": "small", "reward": 2.0},
		} {
			doc["_ts"] = day.Add(time.Duration(i) * time.Minute)
			doc["_block"] = 110000000 - d*20000 - i
			if _, err := collection.InsertOne(ctx, doc); err != nil {
				t.Fatalf("insert benefactor day %d: %v", d, err)
			}
		}
	}

	// History that must stay out: an old day and a cold-start document with an
	// uninitialised block timestamp (the "Mon 1/1/1" card).
	if _, err := collection.InsertOne(ctx, bson.M{
		"_ts": time.Date(2017, 4, 12, 3, 0, 0, 0, time.UTC), "benefactor": "ancient", "reward": 140283.263256,
	}); err != nil {
		t.Fatalf("insert old doc: %v", err)
	}
	if _, err := collection.InsertOne(ctx, bson.M{
		"_ts": time.Time{}, "benefactor": "zero-ts", "reward": 27.551156,
	}); err != nil {
		t.Fatalf("insert zero-ts doc: %v", err)
	}

	got, err := svc.GetBenefactors(ctx)
	if err != nil {
		t.Fatalf("GetBenefactors: %v", err)
	}

	if len(got.Dates) != 10 {
		t.Fatalf("got %d days, want 10", len(got.Dates))
	}

	newest := got.Dates[0]
	if newest.Year != today.Year() || time.Month(newest.Month) != today.Month() || newest.Day != today.Day() {
		t.Errorf("newest day = %d/%d/%d, want today %d/%d/%d",
			newest.Month, newest.Day, newest.Year, int(today.Month()), today.Day(), today.Year())
	}
	// Each day has three ops; reward = (1000+d) + (1+d) + 2.
	wantReward := (1000.0 + 0) + (1.0 + 0) + 2.0
	if newest.Reward != wantReward {
		t.Errorf("newest day reward = %v, want %v", newest.Reward, wantReward)
	}
	if newest.Total != 3 {
		t.Errorf("newest day total = %d, want 3 ops", newest.Total)
	}
	if len(newest.Benefactors) != 2 {
		t.Fatalf("newest day benefactors = %d, want 2", len(newest.Benefactors))
	}
	if newest.Benefactors[0].Benefactor != "big" {
		t.Errorf("benefactors[0] = %q, want the highest reward first (big)", newest.Benefactors[0].Benefactor)
	}
	if newest.Benefactors[1].Benefactor != "small" || newest.Benefactors[1].Count != 2 {
		t.Errorf("benefactors[1] = %+v, want small with 2 ops", newest.Benefactors[1])
	}

	// Days must run newest -> oldest and never reach the excluded history.
	for i := 1; i < len(got.Dates); i++ {
		prev := got.Dates[i-1]
		cur := got.Dates[i]
		if cur.Year > prev.Year || (cur.Year == prev.Year && cur.DayOfYear >= prev.DayOfYear) {
			t.Errorf("dates not strictly newest-first at index %d: %+v then %+v", i, prev, cur)
		}
	}
	if oldest := got.Dates[len(got.Dates)-1]; oldest.Year < today.Year()-1 {
		t.Errorf("oldest returned day = %d/%d/%d, way outside the 10-day window",
			oldest.Month, oldest.Day, oldest.Year)
	}
	for _, date := range got.Dates {
		for _, b := range date.Benefactors {
			if b.Benefactor == "ancient" || b.Benefactor == "zero-ts" {
				t.Fatalf("excluded history leaked into the window: %+v", b)
			}
		}
	}
}

// TestFlagsSnapshotAgainstMongo pins the flags reader: no snapshot is an
// error (never an empty-but-successful page), and a written snapshot maps to
// the API shape with the trimmed fields.
func TestFlagsSnapshotAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	if _, err := svc.GetFlags(ctx); err == nil {
		t.Fatal("GetFlags with no snapshot = nil error, want an explicit failure")
	}

	// Shape written by steemdb-sync's flags refresher.
	snapshot := bson.M{
		"_id": "flags-snapshot",
		"data": bson.A{
			bson.M{
				"author":      "appics-introduce",
				"count":       int64(381430),
				"voter_count": int64(251),
				"post_count":  int64(8234),
				"voters": bson.A{
					bson.M{"voter": "steemcleaners", "count": int64(900)},
				},
			},
		},
		"updated_at": time.Now().UTC(),
	}
	if _, err := svc.db.Collection("status").InsertOne(ctx, snapshot); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	flags, err := svc.GetFlags(ctx)
	if err != nil {
		t.Fatalf("GetFlags: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("got %d flags, want 1", len(flags))
	}

	got := flags[0]
	if got.Author != "appics-introduce" || got.Count != 381430 {
		t.Errorf("flag = %+v, want appics-introduce/381430", got)
	}
	if got.VoterCount != 251 || got.PostCount != 8234 {
		t.Errorf("voter_count/post_count = %d/%d, want 251/8234", got.VoterCount, got.PostCount)
	}
	if got.Voters["steemcleaners"] != 900 {
		t.Errorf("voters = %v, want steemcleaners=900", got.Voters)
	}

	// A stale snapshot must still be served (the refresher overwrites it);
	// prove the reader does not filter on updated_at.
	if _, err := svc.db.Collection("status").UpdateOne(ctx,
		bson.M{"_id": "flags-snapshot"},
		bson.M{"$set": bson.M{"updated_at": time.Now().UTC().AddDate(0, 0, -3)}},
		options.Update(),
	); err != nil {
		t.Fatalf("age snapshot: %v", err)
	}
	if flags, err := svc.GetFlags(ctx); err != nil || len(flags) != 1 {
		t.Errorf("stale snapshot not served: flags=%v err=%v", flags, err)
	}
}

// TestPowerUpTotalAgainstMongo pins the aggregation fix: two deposits of the
// same amount must both count towards `total`. The previous pipeline derived
// total from an $addToSet of amounts, so identical deposits collapsed into one
// and the leaderboard under-reported while `count` stayed correct.
func TestPowerUpTotalAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	collection := svc.db.Collection("vesting_deposit")
	now := time.Now().UTC()
	docs := []bson.M{
		// ecotracker: four deposits, three of them identical.
		{"_ts": now.Add(-1 * time.Hour), "from": "ecotracker", "to": "", "amount": 0.001},
		{"_ts": now.Add(-2 * time.Hour), "from": "ecotracker", "to": "", "amount": 0.001},
		{"_ts": now.Add(-3 * time.Hour), "from": "ecotracker", "to": "", "amount": 0.001},
		{"_ts": now.Add(-4 * time.Hour), "from": "ecotracker", "to": "", "amount": 5.0},
		// delegated: the deposit is credited to `to`, not `from`.
		{"_ts": now.Add(-5 * time.Hour), "from": "sponsor", "to": "delegate", "amount": 2.5},
		// outside the 30-day window
		{"_ts": now.AddDate(0, 0, -40), "from": "ecotracker", "to": "", "amount": 1000.0},
	}
	if _, err := collection.InsertMany(ctx, asDocs(docs)); err != nil {
		t.Fatalf("insert deposits: %v", err)
	}

	got, err := svc.GetPowerUps(ctx, "")
	if err != nil {
		t.Fatalf("GetPowerUps: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d accounts, want 2: %+v", len(got), got)
	}
	if got[0].User != "ecotracker" {
		t.Errorf("first account = %q, want ecotracker (largest total)", got[0].User)
	}
	if got[0].Count != 4 {
		t.Errorf("ecotracker count = %d, want 4", got[0].Count)
	}
	// 3 × 0.001 + 5.0 = 5.003. Summing the distinct-amount set gives 5.001.
	if diff := got[0].Total - 5.003; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("ecotracker total = %v, want 5.003 (duplicate amounts counted)", got[0].Total)
	}
	if got[1].User != "delegate" || got[1].Total != 2.5 {
		t.Errorf("second entry = %+v, want delegate/2.5 (credited to the `to` account)", got[1])
	}

	// The 7-day window must exclude nothing here except the old deposit that
	// was already excluded; a week filter drops nothing but keeps the sums.
	week, err := svc.GetPowerUps(ctx, "week")
	if err != nil {
		t.Fatalf("GetPowerUps(week): %v", err)
	}
	if len(week) != 2 || week[0].Total != 5.003 {
		t.Errorf("week window = %+v, want the same two accounts", week)
	}
}

// TestPendingPostsAgainstMongo pins the scope fix: replies and zero-payout
// posts must not appear on a page titled "Pending Posts".
func TestPendingPostsAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	collection := svc.db.Collection("comment")
	now := time.Now().UTC()
	// Inside the [7d, 6.5d] window: a top-level post with a payout, a reply
	// with a payout, and a top-level post with nothing pending.
	created := now.Add(-160 * time.Hour)
	docs := []bson.M{
		{"_id": "a/p1", "author": "a", "permlink": "p1", "title": "post", "depth": 0, "created": created, "pending_payout_value": 12.5},
		{"_id": "b/re1", "author": "b", "permlink": "re1", "title": "", "depth": 1, "created": created, "pending_payout_value": 99.0},
		{"_id": "c/p2", "author": "c", "permlink": "p2", "title": "declining", "depth": 0, "created": created, "pending_payout_value": 0},
		{"_id": "d/p3", "author": "d", "permlink": "p3", "title": "too old", "depth": 0, "created": now.Add(-200 * time.Hour), "pending_payout_value": 50.0},
	}
	if _, err := collection.InsertMany(ctx, asDocs(docs)); err != nil {
		t.Fatalf("insert comments: %v", err)
	}

	got, err := svc.GetPendingPosts(ctx)
	if err != nil {
		t.Fatalf("GetPendingPosts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d posts, want 1 (the other three are a reply, a zero payout and an out-of-window post): %+v", len(got), got)
	}
	if got[0].Author != "a" || got[0].PendingPayoutValue != 12.5 {
		t.Errorf("pending post = %+v, want a/p1 with 12.5", got[0])
	}
}

// TestCurationMonthlyAgainstMongo pins the monthly window and the permlink cap
// that keeps the response small.
func TestCurationMonthlyAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	collection := svc.db.Collection("curation_reward")
	docs := []bson.M{
		{"curator": "curators", "_ts": time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), "reward": 100.0, "comment_author": "a", "comment_permlink": "p1"},
		{"curator": "curators", "_ts": time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC), "reward": 50.0, "comment_author": "b", "comment_permlink": "p2"},
		// next month — must stay out of the September window
		{"curator": "curators", "_ts": time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC), "reward": 999.0, "comment_author": "c", "comment_permlink": "p3"},
	}
	if _, err := collection.InsertMany(ctx, asDocs(docs)); err != nil {
		t.Fatalf("insert rewards: %v", err)
	}

	got, err := svc.GetCurationLeaderboard(ctx, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "monthly")
	if err != nil {
		t.Fatalf("GetCurationLeaderboard: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d curators, want 1", len(got))
	}
	if got[0].Curator != "curators" || got[0].Count != 2 || got[0].Total != 150.0 {
		t.Errorf("curation row = %+v, want curators/2 ops/150 VESTS", got[0])
	}
}

// TestCurationPermlinkCapAgainstMongo bounds the response payload: a curator
// with far more posts than the cap still returns a full count and total.
func TestCurationPermlinkCapAgainstMongo(t *testing.T) {
	svc, ctx := labsTestService(t)

	collection := svc.db.Collection("curation_reward")
	day := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	docs := make([]bson.M, 0, curatedPermlinksPerCurator+15)
	for i := 0; i < curatedPermlinksPerCurator+15; i++ {
		docs = append(docs, bson.M{
			"curator": "heavy", "_ts": day.Add(time.Duration(i) * time.Minute), "reward": 1.0,
			"comment_author": "author", "comment_permlink": fmt.Sprintf("p%d", i),
		})
	}
	if _, err := collection.InsertMany(ctx, asDocs(docs)); err != nil {
		t.Fatalf("insert rewards: %v", err)
	}

	got, err := svc.GetCurationLeaderboard(ctx, day, "daily")
	if err != nil {
		t.Fatalf("GetCurationLeaderboard: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d curators, want 1", len(got))
	}
	if got[0].Count != curatedPermlinksPerCurator+15 {
		t.Errorf("count = %d, want %d (the cap applies to permlinks only)", got[0].Count, curatedPermlinksPerCurator+15)
	}
	if len(got[0].Permlinks) != curatedPermlinksPerCurator {
		t.Errorf("permlinks = %d, want the cap %d", len(got[0].Permlinks), curatedPermlinksPerCurator)
	}
	if got[0].Total != float64(curatedPermlinksPerCurator+15) {
		t.Errorf("total = %v, want %d", got[0].Total, curatedPermlinksPerCurator+15)
	}
}

// asDocs adapts a typed document slice to the driver's InsertMany signature.
func asDocs(ms []bson.M) []interface{} {
	out := make([]interface{}, 0, len(ms))
	for _, m := range ms {
		out = append(out, m)
	}
	return out
}
