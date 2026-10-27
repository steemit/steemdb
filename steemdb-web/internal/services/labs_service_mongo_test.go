package services

import (
	"context"
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
