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
	Props              *steem.DynamicGlobalProperties `json:"props"`
	LatestBlocks       []models.BlockSummary          `json:"latest_blocks"`
	Stats              *DashboardStats                `json:"stats"`
	NetworkPerformance *models.NetworkPerformance     `json:"network_performance,omitempty"`
	RewardPool         map[string]interface{}         `json:"reward_pool,omitempty"`
	IsFromUpstream     bool                           `json:"is_from_upstream"` // Indicates if data came from upstream API
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
	collection := s.db.Collection("blocks")
	var latestBlock struct {
		BlockNum  uint32    `bson:"block_num"`
		Timestamp time.Time `bson:"timestamp"`
	}

	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"block_num": -1})).Decode(&latestBlock)

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
		if int64(latestBlock.BlockNum) < upstreamProps.HeadBlockNumber-10 {
			useUpstream = true
		}
	}

	var dashboardData *DashboardData
	if useUpstream {
		dashboardData, err = s.getDashboardDataFromUpstream(ctx, upstreamProps)
	} else {
		dashboardData, err = s.getDashboardDataFromLocal(ctx, upstreamProps)
	}

	if err != nil {
		return nil, err
	}

	// Get Network Performance data (always from local database)
	networkPerf, err := s.GetNetworkPerformance(ctx)
	if err != nil {
		s.logger.Warn("Failed to get network performance", utils.Error(err))
	} else {
		dashboardData.NetworkPerformance = networkPerf
	}

	// Get Reward Pool data (always from local database)
	rewardPool, err := s.GetRewardPool(ctx)
	if err != nil {
		s.logger.Warn("Failed to get reward pool", utils.Error(err))
	} else {
		dashboardData.RewardPool = rewardPool
	}

	return dashboardData, nil
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
			Number:           uint32(blockNum),
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
	collection := s.db.Collection("blocks")
	findOptions := options.Find().
		SetSort(bson.M{"block_num": -1}).
		SetLimit(5)

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest blocks: %w", err)
	}
	defer cursor.Close(ctx)

	latestBlocks := make([]models.BlockSummary, 0, 5)
	for cursor.Next(ctx) {
		var block struct {
			BlockNum         uint32    `bson:"block_num"`
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
			Number:           block.BlockNum,
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

// GetNetworkPerformance gets network performance metrics from status collection
func (s *DashboardService) GetNetworkPerformance(ctx context.Context) (*models.NetworkPerformance, error) {
	statusCollection := s.db.Collection("status")

	// Helper function to get status value
	getStatusValue := func(id string) int64 {
		var status struct {
			ID   string `bson:"_id"`
			Data int64  `bson:"data"`
		}
		err := statusCollection.FindOne(ctx, bson.M{"_id": id}).Decode(&status)
		if err != nil {
			return 0
		}
		return status.Data
	}

	tx24h := getStatusValue("transactions-24h")
	tx1h := getStatusValue("transactions-1h")
	op24h := getStatusValue("operations-24h")
	op1h := getStatusValue("operations-1h")

	// Calculate per second rates
	txPerSec24h := float64(tx24h) / 86400.0 // 24 hours = 86400 seconds
	txPerSec1h := float64(tx1h) / 3600.0    // 1 hour = 3600 seconds
	opPerSec24h := float64(op24h) / 86400.0
	opPerSec1h := float64(op1h) / 3600.0

	return &models.NetworkPerformance{
		Transactions24h:       tx24h,
		Transactions1h:        tx1h,
		TransactionsPerSec24h: txPerSec24h,
		TransactionsPerSec1h:  txPerSec1h,
		Operations24h:         op24h,
		Operations1h:          op1h,
		OperationsPerSec24h:   opPerSec24h,
		OperationsPerSec1h:    opPerSec1h,
	}, nil
}

// GetRewardPool gets reward pool data from funds_history collection
func (s *DashboardService) GetRewardPool(ctx context.Context) (map[string]interface{}, error) {
	fundsCollection := s.db.Collection("funds_history")

	var fundsHistory bson.M

	err := fundsCollection.FindOne(
		ctx,
		bson.M{"name": "post"},
		options.FindOne().SetSort(bson.M{"last_update": -1}),
	).Decode(&fundsHistory)

	if err != nil {
		return nil, fmt.Errorf("failed to get reward pool: %w", err)
	}

	// Remove internal fields and convert to map[string]interface{}
	result := make(map[string]interface{})
	for k, v := range fundsHistory {
		if k != "_id" && k != "id" && k != "name" {
			result[k] = v
		}
	}

	return result, nil
}
