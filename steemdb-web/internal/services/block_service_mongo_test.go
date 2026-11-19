package services

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// blockTestService returns a BlockService bound to the integration-test MongoDB
// (STEEMDB_TEST_MONGO_URI, same convention as the labs tests), or skips when no
// database is configured. The steem RPC client is nil on purpose: the two stats
// methods must answer from the local database alone.
func blockTestService(t *testing.T) (*BlockService, context.Context) {
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
		Database: "steemdb_web_block_test",
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

	return NewBlockService(db, nil, logger), ctx
}

// TestOperationStatsAgainstMongo pins the window semantics of the histogram:
// the head block is the upper bound, the range maps to blocks via the 3s slot
// time, ops older than the window are excluded, virtual ops are included, and
// the result is ordered by count desc.
func TestOperationStatsAgainstMongo(t *testing.T) {
	svc, ctx := blockTestService(t)

	head := int64(2000)
	seedBlock(t, ctx, svc, head)

	// One hour == 1200 blocks, so a 1h window over head=2000 is [800, 2000].
	seedOps(t, ctx, svc, []opSeed{
		{blockNum: 800, opType: "vote", count: 3}, // boundary: included by $gte
		{blockNum: 1500, opType: "comment", count: 2},
		{blockNum: 1999, opType: "producer_reward", count: 1, virtual: true},
		{blockNum: 799, opType: "vote", count: 1}, // just outside the window
	})

	stats, err := svc.GetOperationStats(ctx, "1h")
	if err != nil {
		t.Fatalf("GetOperationStats: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("want 3 operation types, got %d: %+v", len(stats), stats)
	}
	want := []struct {
		opType string
		count  int64
		pct    float64
	}{
		{"vote", 3, 50.0},
		{"comment", 2, 33.33},
		{"producer_reward", 1, 16.67},
	}
	for i, w := range want {
		got := stats[i]
		if got.Type != w.opType || got.Count != w.count {
			t.Errorf("pos %d: want %s/%d, got %s/%d", i, w.opType, w.count, got.Type, got.Count)
		}
		if got.Percentage != w.pct {
			t.Errorf("pos %d (%s): want pct %.2f, got %.2f", i, w.opType, w.pct, got.Percentage)
		}
	}

	// The percentages are of the window total (6 ops), so they add up to 100.
	var sum float64
	for _, s := range stats {
		sum += s.Percentage
	}
	if sum < 99.99 || sum > 100.01 {
		t.Errorf("percentages should sum to 100, got %.2f", sum)
	}

	// The default 24h window reaches back past block 799 (head-28800 < 0 -> 0),
	// so the same vote total grows to 4 — proof the range actually moves the window.
	wide, err := svc.GetOperationStats(ctx, "")
	if err != nil {
		t.Fatalf("GetOperationStats(24h default): %v", err)
	}
	if wide[0].Type != "vote" || wide[0].Count != 4 {
		t.Fatalf("default range should include block 799: got %+v", wide[0])
	}
}

// TestOperationStatsEmptyWindow makes sure a window with no operations answers
// with an empty list rather than an error or null.
func TestOperationStatsEmptyWindow(t *testing.T) {
	svc, ctx := blockTestService(t)

	head := int64(200000)
	seedBlock(t, ctx, svc, head)
	// Everything sits far below the 1h window (head-1200).
	seedOps(t, ctx, svc, []opSeed{{blockNum: 1000, opType: "vote", count: 5}})

	stats, err := svc.GetOperationStats(ctx, "1h")
	if err != nil {
		t.Fatalf("GetOperationStats: %v", err)
	}
	if len(stats) != 0 {
		t.Fatalf("want empty result outside the window, got %+v", stats)
	}
}

// TestParseStatsRange covers the accepted syntax and the bounds. It needs no
// database, so it runs in the default (unconfigured) test environment.
func TestParseStatsRange(t *testing.T) {
	ok := map[string]time.Duration{
		"":     defaultStatsRange,
		"1h":   time.Hour,
		"6h":   6 * time.Hour,
		"24h":  24 * time.Hour,
		"7d":   7 * 24 * time.Hour,
		" 12h": 12 * time.Hour,
	}
	for raw, want := range ok {
		got, err := parseStatsRange(raw)
		if err != nil {
			t.Errorf("%q: unexpected error %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("%q: want %s, got %s", raw, want, got)
		}
	}

	for _, raw := range []string{"30m", "30d", "0d", "abc", "1", "-2h", "8d"} {
		if _, err := parseStatsRange(raw); !errors.Is(err, ErrInvalidStatsRange) {
			t.Errorf("%q: want ErrInvalidStatsRange, got %v", raw, err)
		}
	}
}

// TestBlockStatsAgainstMongo pins the totals (estimated count of the whole
// collection) and the one-hour rates (real count over the last 1200 blocks).
func TestBlockStatsAgainstMongo(t *testing.T) {
	svc, ctx := blockTestService(t)

	head := int64(2000)
	headTime := time.Unix(1795000000, 0).UTC()
	seedBlockAt(t, ctx, svc, head, headTime)

	// Inside the one-hour window [800, 2000].
	seedTransactions(t, ctx, svc, []int64{800, 1200, 2000})
	seedOps(t, ctx, svc, []opSeed{
		{blockNum: 800, opType: "vote", count: 3},
		{blockNum: 1500, opType: "comment", count: 2},
	})
	// Outside the window but still in the collection.
	seedTransactions(t, ctx, svc, []int64{100})
	seedOps(t, ctx, svc, []opSeed{{blockNum: 100, opType: "vote", count: 2}})

	stats, err := svc.GetBlockStats(ctx)
	if err != nil {
		t.Fatalf("GetBlockStats: %v", err)
	}

	if stats.LatestBlockNum != head {
		t.Errorf("latest_block_num: want %d, got %d", head, stats.LatestBlockNum)
	}
	if stats.LastIrreversibleNum != head-20 {
		t.Errorf("last_irreversible_num: want %d, got %d", head-20, stats.LastIrreversibleNum)
	}
	if !stats.HeadBlockTime.Equal(headTime) {
		t.Errorf("head_block_time: want %s, got %s", headTime, stats.HeadBlockTime)
	}
	// Estimated counts are exact for these tiny collections.
	if stats.TotalTransactions != 4 || stats.TotalOperations != 7 {
		t.Errorf("totals: want 4 txs / 7 ops, got %d / %d",
			stats.TotalTransactions, stats.TotalOperations)
	}
	if !stats.TotalsEstimated {
		t.Error("totals_estimated must be true: the totals come from collection metadata")
	}
	if stats.TransactionsPerHour != 3 || stats.OperationsPerHour != 5 {
		t.Errorf("per-hour: want 3 txs / 5 ops, got %d / %d",
			stats.TransactionsPerHour, stats.OperationsPerHour)
	}
	if stats.AverageBlockTime != steemBlockInterval.Seconds() {
		t.Errorf("average_block_time: want %v, got %v",
			steemBlockInterval.Seconds(), stats.AverageBlockTime)
	}
}

// TestBlockStatsEmptyCollection documents the error path: without any ingested
// block there is no head to anchor a window on.
func TestBlockStatsEmptyCollection(t *testing.T) {
	svc, ctx := blockTestService(t)

	if _, err := svc.GetBlockStats(ctx); err == nil {
		t.Fatal("want an error for an empty blocks collection, got nil")
	}
}

type opSeed struct {
	blockNum int64
	opType   string
	count    int
	virtual  bool
}

func seedBlock(t *testing.T, ctx context.Context, svc *BlockService, num int64) {
	t.Helper()
	seedBlockAt(t, ctx, svc, num, time.Unix(1795000000, 0).UTC())
}

func seedBlockAt(t *testing.T, ctx context.Context, svc *BlockService, num int64, ts time.Time) {
	t.Helper()
	_, err := svc.db.Collection("blocks").InsertOne(ctx, bson.M{
		"_id":       num,
		"block_num": num,
		"timestamp": ts,
		"witness":   "testwitness",
	})
	if err != nil {
		t.Fatalf("seed block %d: %v", num, err)
	}
}

func seedOps(t *testing.T, ctx context.Context, svc *BlockService, seeds []opSeed) {
	t.Helper()
	docs := make([]interface{}, 0, len(seeds))
	for _, s := range seeds {
		for i := 0; i < s.count; i++ {
			docs = append(docs, bson.M{
				"_id":       s.blockNum*1000 + int64(i),
				"block_num": s.blockNum,
				"op_type":   s.opType,
				"virtual":   s.virtual,
				"trx_index": 0,
				"op_index":  i,
			})
		}
	}
	if len(docs) == 0 {
		return
	}
	if _, err := svc.db.Collection("operations").InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed operations: %v", err)
	}
}

func seedTransactions(t *testing.T, ctx context.Context, svc *BlockService, blockNums []int64) {
	t.Helper()
	docs := make([]interface{}, 0, len(blockNums))
	for i, num := range blockNums {
		docs = append(docs, bson.M{
			"_id":       num*1000 + int64(i),
			"block_num": num,
			"trx_index": i,
		})
	}
	if len(docs) == 0 {
		return
	}
	if _, err := svc.db.Collection("transactions").InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed transactions: %v", err)
	}
}
