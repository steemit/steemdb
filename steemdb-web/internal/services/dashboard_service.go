package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// DashboardService handles dashboard data operations
type DashboardService struct {
	db          *database.MongoDB
	steemClient *steem.Client
	logger      utils.Logger
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(db *database.MongoDB, steemClient *steem.Client, logger utils.Logger) *DashboardService {
	return &DashboardService{
		db:          db,
		steemClient: steemClient,
		logger:      logger,
	}
}

// DashboardData represents all dashboard data
type DashboardData struct {
	Props          *steem.DynamicGlobalProperties `json:"props"`
	LatestBlocks   []models.BlockSummary          `json:"latest_blocks"`
	Stats          *DashboardStats                `json:"stats"`
	IsFromUpstream bool                           `json:"is_from_upstream"` // Indicates if data came from upstream API
}

// DashboardStats represents dashboard statistics
type DashboardStats struct {
	Accounts  int64 `json:"accounts"`
	Comments  int64 `json:"comments"`
	Witnesses int64 `json:"witnesses"`
}

// GetDashboardData gets dashboard data, fetching from upstream if local data is stale
func (s *DashboardService) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	// First, try to get latest block from database
	collection := s.db.Collection("block")
	var latestBlock struct {
		Number    int64     `bson:"number"`
		Timestamp time.Time `bson:"timestamp"`
	}

	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"number": -1})).Decode(&latestBlock)

	// Get current block from upstream to compare
	upstreamProps, err := s.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		s.logger.Error("Failed to get upstream properties", utils.Error(err))
		// Continue with local data if available
	}

	// Determine if we should use upstream data
	useUpstream := false
	if err != nil || upstreamProps == nil {
		// No local data, use upstream
		useUpstream = true
	} else if upstreamProps != nil {
		// Check if local data is stale (more than 10 blocks behind)
		if latestBlock.Number < upstreamProps.HeadBlockNumber-10 {
			useUpstream = true
		}
	}

	if useUpstream {
		return s.getDashboardDataFromUpstream(ctx, upstreamProps)
	}

	// Use local data
	return s.getDashboardDataFromLocal(ctx, upstreamProps)
}

// getDashboardDataFromUpstream fetches dashboard data from upstream API
func (s *DashboardService) getDashboardDataFromUpstream(ctx context.Context, props *steem.DynamicGlobalProperties) (*DashboardData, error) {
	if props == nil {
		var err error
		props, err = s.steemClient.GetDynamicGlobalProperties()
		if err != nil {
			return nil, fmt.Errorf("failed to get dynamic global properties: %w", err)
		}
	}

	// Get latest blocks from upstream
	latestBlocks := make([]models.BlockSummary, 0, 5)
	startBlock := props.HeadBlockNumber
	if startBlock > 5 {
		startBlock = startBlock - 4
	}

	for blockNum := startBlock; blockNum <= props.HeadBlockNumber; blockNum++ {
		block, err := s.steemClient.GetBlock(blockNum)
		if err != nil {
			s.logger.Warn("Failed to fetch block from upstream", utils.Int64("block_num", blockNum), utils.Error(err))
			continue
		}

		opsCount := 0
		for _, tx := range block.Transactions {
			opsCount += len(tx.Operations)
		}

		latestBlocks = append(latestBlocks, models.BlockSummary{
			Number:           blockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: len(block.Transactions),
			OperationCount:   opsCount,
		})
	}

	// Get active witnesses count
	activeWitnesses, err := s.steemClient.GetActiveWitnesses()
	witnessCount := int64(0)
	if err == nil {
		witnessCount = int64(len(activeWitnesses))
	}

	// Try to get account count from database (this is usually accurate even if blocks are behind)
	accountCount := int64(0)
	accountCollection := s.db.Collection("account")
	if count, err := accountCollection.CountDocuments(ctx, bson.M{}); err == nil {
		accountCount = count
	}

	// Try to get comment count from database
	commentCount := int64(0)
	commentCollection := s.db.Collection("comment")
	if count, err := commentCollection.CountDocuments(ctx, bson.M{}); err == nil {
		commentCount = count
	}

	return &DashboardData{
		Props:        props,
		LatestBlocks: latestBlocks,
		Stats: &DashboardStats{
			Accounts:  accountCount,
			Comments:  commentCount,
			Witnesses: witnessCount,
		},
		IsFromUpstream: true,
	}, nil
}

// getDashboardDataFromLocal fetches dashboard data from local database
func (s *DashboardService) getDashboardDataFromLocal(ctx context.Context, props *steem.DynamicGlobalProperties) (*DashboardData, error) {
	// Get latest blocks from database
	collection := s.db.Collection("block")
	findOptions := options.Find().
		SetSort(bson.M{"number": -1}).
		SetLimit(5)

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest blocks: %w", err)
	}
	defer cursor.Close(ctx)

	latestBlocks := make([]models.BlockSummary, 0, 5)
	for cursor.Next(ctx) {
		var block struct {
			Number           int64     `bson:"number"`
			Timestamp        time.Time `bson:"timestamp"`
			Witness          string    `bson:"witness"`
			TransactionCount int       `bson:"transaction_count"`
			OperationCount   int       `bson:"operation_count"`
		}
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		latestBlocks = append(latestBlocks, models.BlockSummary{
			Number:           block.Number,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
		})
	}

	// Get stats from database
	accountCount := int64(0)
	accountCollection := s.db.Collection("account")
	if count, err := accountCollection.CountDocuments(ctx, bson.M{}); err == nil {
		accountCount = count
	}

	commentCount := int64(0)
	commentCollection := s.db.Collection("comment")
	if count, err := commentCollection.CountDocuments(ctx, bson.M{}); err == nil {
		commentCount = count
	}

	witnessCount := int64(0)
	witnessCollection := s.db.Collection("witness")
	if count, err := witnessCollection.CountDocuments(ctx, bson.M{}); err == nil {
		witnessCount = count
	}

	// If props not provided, get from upstream (we still need current witness info)
	if props == nil {
		var err error
		props, err = s.steemClient.GetDynamicGlobalProperties()
		if err != nil {
			s.logger.Warn("Failed to get upstream properties for local data", utils.Error(err))
		}
	}

	return &DashboardData{
		Props:        props,
		LatestBlocks: latestBlocks,
		Stats: &DashboardStats{
			Accounts:  accountCount,
			Comments:  commentCount,
			Witnesses: witnessCount,
		},
		IsFromUpstream: false,
	}, nil
}
