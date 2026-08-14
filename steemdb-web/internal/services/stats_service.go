package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
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

// GetGlobalStats returns aggregate counts from the local database
func (s *StatsService) GetGlobalStats(ctx context.Context) (*GlobalStats, error) {
	stats := &GlobalStats{
		LastUpdate: time.Now(),
	}

	// Count accounts
	if count, err := s.db.Collection("account").CountDocuments(ctx, bson.M{}); err == nil {
		stats.Accounts = count
	} else {
		s.logger.Warn("Failed to count accounts", utils.Error(err))
	}

	// Count blocks
	if count, err := s.db.Collection("blocks").CountDocuments(ctx, bson.M{}); err == nil {
		stats.Blocks = count
	} else {
		s.logger.Warn("Failed to count blocks", utils.Error(err))
	}

	// Count operations
	if count, err := s.db.Collection("operations").CountDocuments(ctx, bson.M{}); err == nil {
		stats.Operations = count
	} else {
		s.logger.Warn("Failed to count operations", utils.Error(err))
	}

	// Count transactions — blocks.transaction_count is currently hardcoded to 0,
	// so we approximate by counting distinct trx_id values in the operations
	// collection. Virtual operations carry an empty trx_id and are excluded.
	txIDs, err := s.db.Collection("operations").Distinct(ctx, "trx_id", bson.M{"trx_id": bson.M{"$ne": ""}})
	if err == nil {
		stats.Transactions = int64(len(txIDs))
	} else {
		s.logger.Warn("Failed to count transactions", utils.Error(err))
	}

	// Get last block number
	var latestBlock struct {
		BlockNum uint32 `bson:"block_num"`
	}
	err = s.db.Collection("blocks").FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"block_num": -1})).Decode(&latestBlock)
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
