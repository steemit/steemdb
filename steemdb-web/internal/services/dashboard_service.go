package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"

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

// GetDashboardData gets dashboard data, fetching from upstream if local data is stale.
// The local probe, the upstream props RPC, network performance and reward pool lookups
// are independent and run concurrently; block/witness/count fetches inside the chosen
// branch run concurrently as well. Every component degrades individually (warn + zero
// value) instead of failing the whole request.
func (s *DashboardService) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	var (
		latestBlockNum int64
		upstreamProps  *steem.DynamicGlobalProperties
		networkPerf    *models.NetworkPerformance
		rewardPool     map[string]interface{}
	)

	// gctx is canceled when the group's work ends; downstream calls must use
	// the original request ctx, which stays live.
	g, gctx := errgroup.WithContext(ctx)

	// Local latest block. _id is the block number; sorting on block_num has no
	// index and degenerates to an in-memory sort over the whole collection.
	g.Go(func() error {
		var latestBlock struct {
			BlockNum uint32 `bson:"block_num"`
		}
		if err := s.db.Collection("blocks").FindOne(gctx, bson.M{}, options.FindOne().SetSort(bson.M{"_id": -1})).Decode(&latestBlock); err == nil {
			latestBlockNum = int64(latestBlock.BlockNum)
		}
		// A failed lookup leaves latestBlockNum 0, which counts as stale below.
		return nil
	})

	g.Go(func() error {
		props, err := s.steemClient.GetDynamicGlobalProperties()
		if err != nil {
			s.logger.Error("Failed to get upstream properties", utils.Error(err))
			return nil // nil props selects the local branch
		}
		upstreamProps = props
		return nil
	})

	g.Go(func() error {
		np, err := s.GetNetworkPerformance(gctx)
		if err != nil {
			s.logger.Warn("Failed to get network performance", utils.Error(err))
			return nil
		}
		networkPerf = np
		return nil
	})

	g.Go(func() error {
		rp, err := s.GetRewardPool(gctx)
		if err != nil {
			s.logger.Warn("Failed to get reward pool", utils.Error(err))
			return nil
		}
		rewardPool = rp
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Use upstream when the local database is missing (0) or more than 10
	// blocks behind the chain head; otherwise serve from local. When the
	// upstream RPC is unavailable, degrade to local data with nil props.
	useUpstream := upstreamProps != nil &&
		(latestBlockNum == 0 || latestBlockNum < upstreamProps.HeadBlockNumber-10)

	var (
		dashboardData *DashboardData
		err           error
	)
	if useUpstream {
		dashboardData, err = s.getDashboardDataFromUpstream(ctx, upstreamProps)
	} else {
		dashboardData, err = s.getDashboardDataFromLocal(ctx, upstreamProps)
	}
	if err != nil {
		return nil, err
	}

	dashboardData.NetworkPerformance = networkPerf
	dashboardData.RewardPool = rewardPool

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

	// Latest blocks from upstream: the head window is fetched concurrently,
	// one goroutine per block, alongside the witness and count lookups.
	latestBlocks := make([]models.BlockSummary, 0, 5)
	startBlock := props.HeadBlockNumber
	if startBlock > 5 {
		startBlock = startBlock - 4
	}
	blockCount := int(props.HeadBlockNumber-startBlock) + 1
	blocks := make([]models.BlockSummary, blockCount)

	var (
		witnessCount int64
		accountCount int64
		commentCount int64
	)

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < blockCount; i++ {
		i := i
		blockNum := startBlock + int64(i)
		g.Go(func() error {
			block, err := s.steemClient.GetBlock(blockNum)
			if err != nil {
				s.logger.Warn("Failed to fetch block from upstream", utils.Int64("block_num", blockNum), utils.Error(err))
				return nil // leaves a zero entry that is dropped below
			}

			opsCount := 0
			for _, tx := range block.Transactions {
				opsCount += len(tx.Operations)
			}

			blocks[i] = models.BlockSummary{
				Number:           uint32(blockNum),
				Timestamp:        block.Timestamp,
				Witness:          block.Witness,
				TransactionCount: len(block.Transactions),
				OperationCount:   opsCount,
			}
			return nil
		})
	}
	g.Go(func() error {
		if activeWitnesses, err := s.steemClient.GetActiveWitnesses(); err == nil {
			witnessCount = int64(len(activeWitnesses))
		}
		return nil
	})
	g.Go(func() error {
		// Display totals: metadata counts, no need for exact scans
		if count, err := s.db.Collection("account").EstimatedDocumentCount(gctx); err == nil {
			accountCount = count
		}
		return nil
	})
	g.Go(func() error {
		if count, err := s.db.Collection("comment").EstimatedDocumentCount(gctx); err == nil {
			commentCount = count
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	for _, b := range blocks {
		if b.Number != 0 {
			latestBlocks = append(latestBlocks, b)
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
		IsFromUpstream: true,
	}, nil
}

// getDashboardDataFromLocal fetches dashboard data from local database
func (s *DashboardService) getDashboardDataFromLocal(ctx context.Context, props *steem.DynamicGlobalProperties) (*DashboardData, error) {
	// Get latest blocks from database. _id is the block number (indexed);
	// sorting on block_num has no index.
	collection := s.db.Collection("blocks")
	findOptions := options.Find().
		SetSort(bson.M{"_id": -1}).
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

	// Stats totals in parallel (display totals: metadata counts)
	var (
		accountCount int64
		commentCount int64
		witnessCount int64
	)
	var wg sync.WaitGroup
	count := func(dst *int64, coll string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, err := s.db.Collection(coll).EstimatedDocumentCount(ctx); err == nil {
				*dst = n
			}
		}()
	}
	count(&accountCount, "account")
	count(&commentCount, "comment")
	count(&witnessCount, "witness")
	wg.Wait()

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
