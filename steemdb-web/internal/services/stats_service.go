package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// StatsService handles global statistics operations
type StatsService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger

	txRefreshing atomic.Bool
}

// NewStatsService creates a new stats service
func NewStatsService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *StatsService {
	return &StatsService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// GlobalStats represents global blockchain statistics
type GlobalStats struct {
	Accounts     int64     `json:"accounts"`
	Blocks       int64     `json:"blocks"`
	Transactions int64     `json:"transactions"`
	Operations   int64     `json:"operations"`
	Witnesses    int64     `json:"witnesses"`
	LastBlock    int64     `json:"last_block"`
	LastUpdate   time.Time `json:"last_update"`
}

// txCacheMaxAge bounds how often the background refresh re-runs.
const txCacheMaxAge = 24 * time.Hour

// txStatsCache is the persisted shape of the transactions counter in the
// stats_cache collection. An exact distinct-trx_id count over the operations
// collection is a multi-minute index scan at 400M documents, so the value is
// computed in the background and served stale-while-revalidate.
type txStatsCache struct {
	Value     int64     `bson:"value"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// serveTxCount returns the cached transactions counter and kicks off a
// background refresh when the value is missing or older than txCacheMaxAge.
// The request path never waits for the computation.
func (s *StatsService) serveTxCount(ctx context.Context) int64 {
	var cached txStatsCache
	err := s.db.Collection("stats_cache").FindOne(ctx, bson.M{"_id": "transactions"}).Decode(&cached)
	switch {
	case err == nil && time.Since(cached.UpdatedAt) < txCacheMaxAge:
		return cached.Value
	case err == nil:
		// stale value: serve it, refresh in the background
		s.maybeRefreshTxCount()
		return cached.Value
	default:
		// no cached value yet: compute in the background, report 0 for now
		s.maybeRefreshTxCount()
		return 0
	}
}

// maybeRefreshTxCount starts the background recompute at most once at a time.
func (s *StatsService) maybeRefreshTxCount() {
	if !s.txRefreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.txRefreshing.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		start := time.Now()
		// Virtual operations carry an empty trx_id and are excluded; every
		// real transaction has exactly one op with op_index 0, so grouping on
		// trx_id (equivalently) counts transactions. allowDiskUse because the
		// $group hash exceeds the 100MB stage limit at this scale unless the
		// planner rewrites it to a DISTINCT_SCAN.
		cursor, err := s.db.Collection("operations").Aggregate(ctx, mongo.Pipeline{
			{bson.E{Key: "$match", Value: bson.M{"trx_id": bson.M{"$ne": ""}}}},
			{bson.E{Key: "$group", Value: bson.M{"_id": "$trx_id"}}},
			{bson.E{Key: "$count", Value: "n"}},
		}, options.Aggregate().SetAllowDiskUse(true))
		if err != nil {
			s.logger.Error("Failed to count transactions (aggregate)", utils.Error(err))
			return
		}
		var res []struct {
			N int64 `bson:"n"`
		}
		if err := cursor.All(ctx, &res); err != nil {
			s.logger.Error("Failed to count transactions (cursor)", utils.Error(err))
			return
		}
		n := int64(0)
		if len(res) > 0 {
			n = res[0].N
		}

		_, err = s.db.Collection("stats_cache").UpdateOne(
			ctx,
			bson.M{"_id": "transactions"},
			bson.M{"$set": bson.M{"value": n, "updated_at": time.Now()}},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			s.logger.Error("Failed to persist transactions cache", utils.Error(err))
			return
		}
		s.logger.Info(fmt.Sprintf("Refreshed transactions count: %d in %s", n, time.Since(start).Round(time.Second)))
	}()
}

// GetGlobalStats returns aggregate counts from the local database
func (s *StatsService) GetGlobalStats(ctx context.Context) (*GlobalStats, error) {
	stats := &GlobalStats{
		LastUpdate: time.Now(),
	}

	// Count accounts (small collection, exact count is cheap)
	if count, err := s.db.Collection("account").CountDocuments(ctx, bson.M{}); err == nil {
		stats.Accounts = count
	} else {
		s.logger.Warn("Failed to count accounts", utils.Error(err))
	}

	// Count blocks/operations: exact counts scan the whole _id index (blocks:
	// ~10s at 20M docs; operations: minutes at 400M), so use the collection
	// metadata counts — these are whole-collection totals where an exact
	// count adds nothing for display purposes.
	if count, err := s.db.Collection("blocks").EstimatedDocumentCount(ctx); err == nil {
		stats.Blocks = count
	} else {
		s.logger.Warn("Failed to count blocks", utils.Error(err))
	}
	if count, err := s.db.Collection("operations").EstimatedDocumentCount(ctx); err == nil {
		stats.Operations = count
	} else {
		s.logger.Warn("Failed to count operations", utils.Error(err))
	}

	// Count transactions: blocks.transaction_count is hardcoded to 0, so the
	// value comes from the cached background computation (see refreshTxCount).
	stats.Transactions = s.serveTxCount(ctx)

	// Get last block number (_id is the block number; sorting on block_num
	// has no index and degenerates to a full in-memory sort)
	var latestBlock struct {
		BlockNum uint32 `bson:"block_num"`
	}
	err := s.db.Collection("blocks").FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"_id": -1})).Decode(&latestBlock)
	if err == nil {
		stats.LastBlock = int64(latestBlock.BlockNum)
	} else {
		s.logger.Warn("Failed to get latest block", utils.Error(err))
	}

	// Get witness count from active witnesses RPC
	if activeWitnesses, err := s.steemClient.GetActiveWitnesses(); err == nil {
		stats.Witnesses = int64(len(activeWitnesses))
	} else {
		s.logger.Warn("Failed to get active witnesses", utils.Error(err))
	}

	return stats, nil
}

// GetBlockchainProps returns the dynamic global properties from steem RPC
func (s *StatsService) GetBlockchainProps(ctx context.Context) (*steem.DynamicGlobalProperties, error) {
	props, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return nil, fmt.Errorf("failed to get blockchain properties: %w", err)
	}
	if props == nil {
		return nil, fmt.Errorf("blockchain properties not available")
	}
	return props, nil
}
