package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// WitnessService handles witness monitoring and data collection
type WitnessService struct {
	config *utils.Config
	db     *database.MongoDB
	steem  *steem.Client
	logger utils.Logger

	scheduler   *cron.Cron
	missesCache map[string]int
	mutex       sync.RWMutex
}

// NewWitnessService creates a new witness service
func NewWitnessService(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *steem.Client,
	logger utils.Logger,
) *WitnessService {
	return &WitnessService{
		config:      config,
		db:          db,
		steem:       steemClient,
		logger:      logger,
		scheduler:   cron.New(),
		missesCache: make(map[string]int),
	}
}

// Start starts the witness service
func (w *WitnessService) Start(ctx context.Context) error {
	w.logger.Info("Starting witness service")

	// Schedule witness update job (every minute)
	_, err := w.scheduler.AddFunc("@every 1m", func() {
		if err := w.updateWitnesses(ctx); err != nil {
			w.logger.Error("Witness update failed", utils.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule witness job: %w", err)
	}

	// Schedule witness miss check job (every 10 seconds)
	_, err = w.scheduler.AddFunc("@every 10s", func() {
		if err := w.checkWitnessMisses(ctx); err != nil {
			w.logger.Error("Witness miss check failed", utils.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule witness miss job: %w", err)
	}

	// Start scheduler
	w.scheduler.Start()

	// Run initial update
	go func() {
		if err := w.updateWitnesses(ctx); err != nil {
			w.logger.Error("Initial witness update failed", utils.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	
	// Stop scheduler
	w.scheduler.Stop()
	return nil
}

// updateWitnesses updates witness information
func (w *WitnessService) updateWitnesses(ctx context.Context) error {
	w.logger.Debug("Updating witnesses")
	startTime := time.Now()

	// Get witnesses from blockchain
	witnesses, err := w.steem.GetWitnessesByVote(ctx, "", 100)
	if err != nil {
		return fmt.Errorf("failed to get witnesses: %w", err)
	}

	w.logger.Debug("Processing witnesses", utils.Int("count", len(witnesses)))

	// Clear existing witnesses
	collection := w.db.Collection("witness")
	if _, err := collection.DeleteMany(ctx, map[string]interface{}{}); err != nil {
		w.logger.Warn("Failed to clear existing witnesses", utils.Error(err))
	}

	// Process witnesses
	var witnessOps []mongo.WriteModel
	var historyOps []mongo.WriteModel

	scanTime := time.Now()
	today := time.Now().Truncate(24 * time.Hour)

	for _, witness := range witnesses {
		// Process witness data
		processedWitness := w.processWitnessData(&witness, scanTime)

		// Witness update operation
		witnessOps = append(witnessOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{"_id": witness.Owner}).
			SetUpdate(map[string]interface{}{"$set": processedWitness}).
			SetUpsert(true))

		// Witness history snapshot
		snapshot := w.createWitnessSnapshot(processedWitness, today)
		historyOps = append(historyOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{
				"_id": fmt.Sprintf("%s|%s", witness.Owner, today.Format("20060102")),
			}).
			SetUpdate(map[string]interface{}{"$set": snapshot}).
			SetUpsert(true))
	}

	// Execute bulk operations
	if len(witnessOps) > 0 {
		if err := w.db.BulkWrite(ctx, "witness", witnessOps); err != nil {
			return fmt.Errorf("failed to bulk write witnesses: %w", err)
		}
	}

	if len(historyOps) > 0 {
		if err := w.db.BulkWrite(ctx, "witness_history", historyOps); err != nil {
			return fmt.Errorf("failed to bulk write witness history: %w", err)
		}
	}

	duration := time.Since(startTime)
	w.logger.Debug("Witness update completed",
		utils.Int("witnesses", len(witnesses)),
		utils.Duration("duration", duration),
	)

	return nil
}

// processWitnessData processes raw witness data from blockchain
func (w *WitnessService) processWitnessData(witness *steem.Witness, scanTime time.Time) *database.Witness {
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
		ID:                     witness.Owner,
		Owner:                  witness.Owner,
		CreatedTime:            witness.CreatedTime.Time,
		URL:                    witness.URL,
		Votes:                  votes,
		VirtualLastUpdate:      virtualLastUpdate,
		VirtualPosition:        virtualPosition,
		VirtualScheduledTime:   virtualScheduledTime,
		TotalMissed:            witness.TotalMissed,
		LastAslot:              witness.LastAslot,
		LastConfirmedBlockNum:  witness.LastConfirmedBlockNum,
		SigningKey:             witness.SigningKey,
		Props:                  propsMap,
		SBDExchangeRate:        exchangeRateMap,
		LastSBDExchangeUpdate:  witness.LastSBDExchangeUpdate.Time,
		LastWork:               witness.LastWork,
		RunningVersion:         witness.RunningVersion,
		HardforkVersionVote:    witness.HardforkVersionVote,
		HardforkTimeVote:       witness.HardforkTimeVote.Time,
	}
}

// createWitnessSnapshot creates a historical snapshot of witness data
func (w *WitnessService) createWitnessSnapshot(witness *database.Witness, date time.Time) *database.WitnessHistory {
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
func (w *WitnessService) checkWitnessMisses(ctx context.Context) error {
	w.logger.Debug("Checking witness misses")

	// Get witnesses from blockchain
	witnesses, err := w.steem.GetWitnessesByVote(ctx, "", 100)
	if err != nil {
		return fmt.Errorf("failed to get witnesses for miss check: %w", err)
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	var missOps []mongo.WriteModel

	for _, witness := range witnesses {
		owner := witness.Owner
		currentMissed := witness.TotalMissed

		// Check if we have a cached value
		if cachedMissed, exists := w.missesCache[owner]; exists {
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

				w.logger.Info("Witness missed blocks",
					utils.String("witness", owner),
					utils.Int("increase", increase),
					utils.Int("total", currentMissed),
				)
			}
		}

		// Update cache
		w.missesCache[owner] = currentMissed
	}

	// Execute bulk operations for misses
	if len(missOps) > 0 {
		if err := w.db.BulkWrite(ctx, "witness_misses", missOps); err != nil {
			return fmt.Errorf("failed to bulk write witness misses: %w", err)
		}
	}

	return nil
}

