package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/utils"
)

// CronTabService handles scheduled tasks (single goroutine)
type CronTabService struct {
	config         *utils.Config
	db             *database.MongoDB
	steem          *utils.SteemClient
	logger         utils.Logger
	blockSync      *BlockSyncService
	accountUpdater *AccountUpdater

	syncReady   bool
	mutex       sync.RWMutex
	missesCache map[string]int
	missesMutex sync.RWMutex
}

// NewCronTabService creates a new cron tab service
func NewCronTabService(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *utils.SteemClient,
	logger utils.Logger,
	blockSync *BlockSyncService,
) *CronTabService {
	accountUpdater := NewAccountUpdater(config, db, steemClient, logger)

	return &CronTabService{
		config:         config,
		db:             db,
		steem:          steemClient,
		logger:         logger,
		blockSync:      blockSync,
		accountUpdater: accountUpdater,
		syncReady:      false,
		missesCache:    make(map[string]int),
	}
}

// Start starts the cron tab service (single goroutine)
func (c *CronTabService) Start(ctx context.Context) error {
	c.logger.Info("Starting cron tab service")

	// Wait for sync to catch up
	c.waitForSyncReady(ctx)

	// Start cron jobs in single goroutine
	go func() {
		c.runCronJobs(ctx)
	}()

	return nil
}

// waitForSyncReady waits for Block Sync to catch up with the latest block
func (c *CronTabService) waitForSyncReady(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	c.logger.Info("Waiting for block sync to catch up...")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if sync has caught up
			if c.isSyncCaughtUp(ctx) {
				c.mutex.Lock()
				c.syncReady = true
				c.mutex.Unlock()
				c.logger.Info("Block sync caught up, starting cron jobs")
				return
			}
		}
	}
}

// isSyncCaughtUp checks if sync has caught up with the latest block
func (c *CronTabService) isSyncCaughtUp(ctx context.Context) bool {
	// Check if Block Sync service reports it's caught up
	if c.blockSync != nil && c.blockSync.IsSyncCaughtUp() {
		return true
	}

	// Fallback: check if last processed block is close to head block
	props, err := c.steem.GetDynamicGlobalProperties(ctx)
	if err != nil {
		c.logger.Debug("Failed to get dynamic global properties for sync check", utils.Error(err))
		return false
	}

	headBlock := int64(props.LastIrreversibleBlockNum)
	lastBlock, err := c.db.GetLastProcessedBlock(ctx)
	if err != nil {
		c.logger.Debug("Failed to get last processed block", utils.Error(err))
		return false
	}

	// Consider caught up if within 10 blocks
	caughtUp := (headBlock - lastBlock) <= 10
	return caughtUp
}

// runCronJobs runs all cron jobs (single goroutine)
func (c *CronTabService) runCronJobs(ctx context.Context) {
	// Account update: every 6 hours
	accountUpdateTicker := time.NewTicker(6 * time.Hour)
	defer accountUpdateTicker.Stop()

	// Hourly stats: every hour
	hourlyStatsTicker := time.NewTicker(1 * time.Hour)
	defer hourlyStatsTicker.Stop()

	// Daily aggregation: every 24 hours
	dailyAggregationTicker := time.NewTicker(24 * time.Hour)
	defer dailyAggregationTicker.Stop()

	// Fund history update: every hour
	fundHistoryTicker := time.NewTicker(1 * time.Hour)
	defer fundHistoryTicker.Stop()

	// Witness update: every minute
	witnessUpdateTicker := time.NewTicker(1 * time.Minute)
	defer witnessUpdateTicker.Stop()

	// Witness miss check: every 10 seconds
	witnessMissTicker := time.NewTicker(10 * time.Second)
	defer witnessMissTicker.Stop()

	// Initial run after 1 minute
	initialTicker := time.NewTicker(1 * time.Minute)
	defer initialTicker.Stop()

	initialRun := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-initialTicker.C:
			if !initialRun {
				initialRun = true
				initialTicker.Stop()
				// Run initial account update
				if err := c.accountUpdater.UpdateAccounts(ctx); err != nil {
					c.logger.Error("Error in initial account update", utils.Error(err))
				}
			}
		case <-accountUpdateTicker.C:
			c.logger.Info("Running scheduled account update")
			if err := c.accountUpdater.UpdateAccounts(ctx); err != nil {
				c.logger.Error("Error updating accounts", utils.Error(err))
			}
		case <-hourlyStatsTicker.C:
			c.logger.Info("Running hourly stats update")
			if err := c.updateHourlyStats(ctx); err != nil {
				c.logger.Error("Error updating hourly stats", utils.Error(err))
			}
		case <-dailyAggregationTicker.C:
			c.logger.Info("Running daily aggregation (30 days)")
			if err := c.calculate30DayAggregations(ctx); err != nil {
				c.logger.Error("Error calculating 30-day aggregations", utils.Error(err))
			}
		case <-fundHistoryTicker.C:
			c.logger.Info("Running fund history update")
			if err := c.updateFundHistory(ctx); err != nil {
				c.logger.Error("Error updating fund history", utils.Error(err))
			}
		case <-witnessUpdateTicker.C:
			c.logger.Debug("Running witness update")
			if err := c.updateWitnesses(ctx); err != nil {
				c.logger.Error("Error updating witnesses", utils.Error(err))
			}
		case <-witnessMissTicker.C:
			c.logger.Debug("Running witness miss check")
			if err := c.checkWitnessMisses(ctx); err != nil {
				c.logger.Error("Error checking witness misses", utils.Error(err))
			}
		}
	}
}

// updateHourlyStats updates hourly statistics
func (c *CronTabService) updateHourlyStats(ctx context.Context) error {
	now := time.Now()
	hourIndex := now.Hour()
	dateIndex := now.Format("2006-01-02")

	// Get operations from the last hour
	startTime := now.Add(-1 * time.Hour)

	collection := c.db.Collection("operations")

	// Aggregate operations by type for the last hour
	pipeline := mongo.Pipeline{
		{bson.E{Key: "$match", Value: bson.D{
			{Key: "block_time", Value: bson.D{
				{Key: "$gte", Value: startTime},
				{Key: "$lt", Value: now},
			}},
		}}},
		{bson.E{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$op_type"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "unique_accounts", Value: bson.D{{Key: "$addToSet", Value: "$primary_account"}}},
		}}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return fmt.Errorf("failed to aggregate hourly stats: %w", err)
	}
	defer cursor.Close(ctx)

	// Update operation_stats
	statsCollection := c.db.Collection("operation_stats")
	for cursor.Next(ctx) {
		var result struct {
			ID             string   `bson:"_id"`
			Count          int64    `bson:"count"`
			UniqueAccounts []string `bson:"unique_accounts"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}

		statsID := fmt.Sprintf("%s_%s_%d", result.ID, dateIndex, hourIndex)
		stats := &database.OperationStats{
			ID:             statsID,
			OpType:         result.ID,
			DateIndex:      dateIndex,
			HourIndex:      hourIndex,
			Count:          result.Count,
			UniqueAccounts: int64(len(result.UniqueAccounts)),
			UpdatedAt:      now,
		}

		filter := bson.D{{Key: "_id", Value: statsID}}
		update := bson.D{{Key: "$set", Value: stats}}
		opts := options.Update().SetUpsert(true)

		if _, err := statsCollection.UpdateOne(ctx, filter, update, opts); err != nil {
			c.logger.Error("Failed to update operation stats",
				utils.String("op_type", result.ID),
				utils.Error(err),
			)
		}
	}

	return nil
}

// calculate30DayAggregations calculates 30-day aggregations
func (c *CronTabService) calculate30DayAggregations(ctx context.Context) error {
	now := time.Now()
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)

	// Calculate global stats
	globalStats := &database.GlobalStats{
		ID:            "current",
		LastBlockTime: now,
		UpdatedAt:     now,
	}

	// Count total accounts
	accountsCollection := c.db.Collection("accounts")
	accountCount, err := accountsCollection.CountDocuments(ctx, bson.D{})
	if err == nil {
		globalStats.TotalAccounts = accountCount
	}

	// Count total posts (comments with empty parent_author)
	commentsCollection := c.db.Collection("comments")
	postCount, err := commentsCollection.CountDocuments(ctx, bson.D{
		{Key: "parent_author", Value: ""},
	})
	if err == nil {
		globalStats.TotalPosts = postCount
	}

	// Count total comments
	commentCount, err := commentsCollection.CountDocuments(ctx, bson.D{})
	if err == nil {
		globalStats.TotalComments = commentCount
	}

	// Count operations in the last 30 days
	operationsCollection := c.db.Collection("operations")

	// Count transfers
	transferCount, err := operationsCollection.CountDocuments(ctx, bson.D{
		{Key: "op_type", Value: "transfer"},
		{Key: "block_time", Value: bson.D{{Key: "$gte", Value: thirtyDaysAgo}}},
	})
	if err == nil {
		globalStats.TotalTransfers = transferCount
	}

	// Count votes
	voteCount, err := operationsCollection.CountDocuments(ctx, bson.D{
		{Key: "op_type", Value: "vote"},
		{Key: "block_time", Value: bson.D{{Key: "$gte", Value: thirtyDaysAgo}}},
	})
	if err == nil {
		globalStats.TotalVotes = voteCount
	}

	// Get last processed block
	lastBlock, err := c.db.GetLastProcessedBlock(ctx)
	if err == nil {
		globalStats.LastBlockNum = lastBlock
	}

	// Update global_stats
	statsCollection := c.db.Collection("global_stats")
	filter := bson.D{{Key: "_id", Value: "current"}}
	update := bson.D{{Key: "$set", Value: globalStats}}
	opts := options.Update().SetUpsert(true)

	if _, err := statsCollection.UpdateOne(ctx, filter, update, opts); err != nil {
		return fmt.Errorf("failed to update global stats: %w", err)
	}

	c.logger.Info("Updated 30-day aggregations",
		utils.Int64("total_accounts", globalStats.TotalAccounts),
		utils.Int64("total_posts", globalStats.TotalPosts),
		utils.Int64("total_comments", globalStats.TotalComments),
		utils.Int64("total_transfers", globalStats.TotalTransfers),
		utils.Int64("total_votes", globalStats.TotalVotes),
	)

	return nil
}

// updateFundHistory updates reward fund history
func (c *CronTabService) updateFundHistory(ctx context.Context) error {
	c.logger.Debug("Updating fund history")

	fund, err := c.steem.GetRewardFund(ctx, "post")
	if err != nil {
		return fmt.Errorf("failed to get reward fund: %w", err)
	}

	fundHistory := &database.FundsHistory{
		ID:                  fmt.Sprintf("post|%d", time.Now().Unix()),
		Name:                fund.Name,
		RewardBalance:       utils.ParseAmountValue(fund.RewardBalance),
		RecentClaims:        utils.ParseFloat64Value(fund.RecentClaims),
		ContentConstant:     utils.ParseFloat64Value(fund.ContentConstant),
		PercentCuration:     fund.PercentCurationRewards,
		PercentContent:      fund.PercentContentRewards,
		LastUpdate:          utils.ToTime(fund.LastUpdate),
		AuthorRewardCurve:   fund.AuthorRewardCurve,
		CurationRewardCurve: fund.CurationRewardCurve,
	}

	collection := c.db.Collection("funds_history")
	_, err = collection.InsertOne(ctx, fundHistory)
	if err != nil {
		return fmt.Errorf("failed to save fund history: %w", err)
	}

	return nil
}

// updateWitnesses updates witness information
func (c *CronTabService) updateWitnesses(ctx context.Context) error {
	c.logger.Debug("Updating witnesses")
	startTime := time.Now()

	// Get witnesses from blockchain
	witnesses, err := c.steem.GetWitnessesByVote(ctx, "", 100)
	if err != nil {
		return fmt.Errorf("failed to get witnesses: %w", err)
	}

	c.logger.Debug("Processing witnesses", utils.Int("count", len(witnesses)))

	// Clear existing witnesses
	collection := c.db.Collection("witness")
	if _, err := collection.DeleteMany(ctx, map[string]interface{}{}); err != nil {
		c.logger.Warn("Failed to clear existing witnesses", utils.Error(err))
	}

	// Process witnesses
	var witnessOps []mongo.WriteModel
	var historyOps []mongo.WriteModel

	scanTime := time.Now()
	today := time.Now().Truncate(24 * time.Hour)

	for _, witness := range witnesses {
		// Process witness data
		processedWitness := c.processWitnessData(&witness, scanTime)

		// Witness update operation
		witnessOps = append(witnessOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{"_id": witness.Owner}).
			SetUpdate(map[string]interface{}{"$set": processedWitness}).
			SetUpsert(true))

		// Witness history snapshot
		snapshot := c.createWitnessSnapshot(processedWitness, today)
		historyOps = append(historyOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{
				"_id": fmt.Sprintf("%s|%s", witness.Owner, today.Format("20060102")),
			}).
			SetUpdate(map[string]interface{}{"$set": snapshot}).
			SetUpsert(true))
	}

	// Execute bulk operations
	if len(witnessOps) > 0 {
		if err := c.db.BulkWrite(ctx, "witness", witnessOps); err != nil {
			return fmt.Errorf("failed to bulk write witnesses: %w", err)
		}
	}

	if len(historyOps) > 0 {
		if err := c.db.BulkWrite(ctx, "witness_history", historyOps); err != nil {
			return fmt.Errorf("failed to bulk write witness history: %w", err)
		}
	}

	duration := time.Since(startTime)
	c.logger.Debug("Witness update completed",
		utils.Int("witnesses", len(witnesses)),
		utils.Duration("duration", duration),
	)

	return nil
}

// processWitnessData processes raw witness data from blockchain
func (c *CronTabService) processWitnessData(witness *utils.Witness, scanTime time.Time) *database.Witness {
	// Parse numeric values
	votes := utils.ParseFloat64Value(witness.Votes)
	virtualLastUpdate := utils.ParseFloat64Value(witness.VirtualLastUpdate)
	virtualPosition := utils.ParseFloat64Value(witness.VirtualPosition)
	virtualScheduledTime := utils.ParseFloat64Value(witness.VirtualScheduledTime)

	// Convert props to map
	propsMap := map[string]interface{}{
		"account_creation_fee": witness.Props.AccountCreationFee,
		"maximum_block_size":   witness.Props.MaximumBlockSize,
		"sbd_interest_rate":    witness.Props.SBDInterestRate,
	}

	// Convert SBD exchange rate to map
	exchangeRateMap := map[string]interface{}{
		"base":  witness.SBDExchangeRate.Base,
		"quote": witness.SBDExchangeRate.Quote,
	}

	return &database.Witness{
		ID:                    witness.Owner,
		Owner:                 witness.Owner,
		CreatedTime:           utils.ToTime(witness.CreatedTime),
		URL:                   witness.URL,
		Votes:                 votes,
		VirtualLastUpdate:     virtualLastUpdate,
		VirtualPosition:       virtualPosition,
		VirtualScheduledTime:  virtualScheduledTime,
		TotalMissed:           witness.TotalMissed,
		LastAslot:             witness.LastAslot,
		LastConfirmedBlockNum: witness.LastConfirmedBlockNum,
		SigningKey:            witness.SigningKey,
		Props:                 propsMap,
		SBDExchangeRate:       exchangeRateMap,
		LastSBDExchangeUpdate: utils.ToTime(witness.LastSBDExchangeUpdate),
		LastWork:              witness.LastWork,
		RunningVersion:        witness.RunningVersion,
		HardforkVersionVote:   witness.HardforkVersionVote,
		HardforkTimeVote:      utils.ToTime(witness.HardforkTimeVote),
	}
}

// createWitnessSnapshot creates a historical snapshot of witness data
func (c *CronTabService) createWitnessSnapshot(witness *database.Witness, date time.Time) *database.WitnessHistory {
	return &database.WitnessHistory{
		ID:      fmt.Sprintf("%s|%s", witness.Owner, date.Format("20060102")),
		Owner:   witness.Owner,
		Date:    date,
		Votes:   witness.Votes,
		Missed:  witness.TotalMissed,
		Created: time.Now(),
	}
}

// checkWitnessMisses checks for witness misses and records them
func (c *CronTabService) checkWitnessMisses(ctx context.Context) error {
	c.logger.Debug("Checking witness misses")

	// Get witnesses from blockchain
	witnesses, err := c.steem.GetWitnessesByVote(ctx, "", 100)
	if err != nil {
		return fmt.Errorf("failed to get witnesses for miss check: %w", err)
	}

	c.missesMutex.Lock()
	defer c.missesMutex.Unlock()

	var missOps []mongo.WriteModel

	for _, witness := range witnesses {
		owner := witness.Owner
		currentMissed := witness.TotalMissed

		// Check if we have a cached value
		if cachedMissed, exists := c.missesCache[owner]; exists {
			// Has the miss count increased?
			if currentMissed > cachedMissed {
				increase := currentMissed - cachedMissed

				// Record the miss event
				missEvent := &database.WitnessMiss{
					ID:       fmt.Sprintf("%s|%d", owner, time.Now().Unix()),
					Date:     time.Now(),
					Witness:  owner,
					Increase: increase,
					Total:    currentMissed,
				}

				missOps = append(missOps, mongo.NewInsertOneModel().SetDocument(missEvent))

				c.logger.Info("Witness missed blocks",
					utils.String("witness", owner),
					utils.Int("increase", increase),
					utils.Int("total", currentMissed),
				)
			}
		}

		// Update cache
		c.missesCache[owner] = currentMissed
	}

	// Execute bulk operations for misses
	if len(missOps) > 0 {
		if err := c.db.BulkWrite(ctx, "witness_misses", missOps); err != nil {
			return fmt.Errorf("failed to bulk write witness misses: %w", err)
		}
	}

	return nil
}
