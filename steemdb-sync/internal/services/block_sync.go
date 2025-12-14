package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemdb/sync/internal/blockchain"
	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// BlockSyncService handles blockchain synchronization (single goroutine)
type BlockSyncService struct {
	config *utils.Config
	db     *database.MongoDB
	steem  *steem.Client
	logger utils.Logger

	// State
	lastBlock int64
	mutex     sync.RWMutex

	// Processor
	operationProcessor *blockchain.OperationProcessor

	// Batch buffers
	blockBuffer []*steem.Block
	bufferMutex sync.Mutex

	// Statistics
	stats *SyncStats

	// Sync status (for CronTab to check)
	syncCaughtUp bool
	syncMutex    sync.RWMutex
}

// SyncStats holds synchronization statistics
type SyncStats struct {
	BlocksProcessed     int64
	OperationsProcessed int64
	ErrorCount          int64
	StartTime           time.Time
	LastBlockTime       time.Time
	mutex               sync.RWMutex
}

// NewBlockSyncService creates a new block sync service
func NewBlockSyncService(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *steem.Client,
	logger utils.Logger,
) *BlockSyncService {
	return &BlockSyncService{
		config:      config,
		db:          db,
		steem:       steemClient,
		logger:      logger,
		blockBuffer: make([]*steem.Block, 0, config.Sync.BlockBatchSize),
		stats: &SyncStats{
			StartTime: time.Now(),
		},
		syncCaughtUp: false,
	}
}

// Start starts the block synchronization service (single goroutine)
func (s *BlockSyncService) Start(ctx context.Context) error {
	s.logger.Info("Starting block sync service (single goroutine)")

	// Load last processed block
	if err := s.loadLastBlock(ctx); err != nil {
		return fmt.Errorf("failed to load last block: %w", err)
	}

	// Initialize operation processor
	s.operationProcessor = blockchain.NewOperationProcessor(s.db, s.logger)

	var wg sync.WaitGroup

	// Start single goroutine sync loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.syncLoop(ctx)
	}()

	// Start statistics reporter
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.statsReporter(ctx)
	}()

	// Start batch writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.batchWriter(ctx)
	}()

	// Start account operations buffer flusher
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.flushAccountOperationsBuffer(ctx)
	}()

	wg.Wait()
	return nil
}

// loadLastBlock loads the last processed block from database
func (s *BlockSyncService) loadLastBlock(ctx context.Context) error {
	lastBlock, err := s.db.GetLastProcessedBlock(ctx)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	s.lastBlock = lastBlock
	s.mutex.Unlock()

	s.logger.Info("Loaded last processed block", utils.Int64("block", lastBlock))
	return nil
}

// syncLoop is the main sync loop (single goroutine, serial processing)
func (s *BlockSyncService) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.Sync.BlockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get blockchain head
			props, err := s.steem.GetDynamicGlobalProperties(ctx)
			if err != nil {
				s.logger.Error("Error getting dynamic global properties", utils.Error(err))
				s.incrementErrorCount()
				continue
			}

			headBlock := props.LastIrreversibleBlockNum

			s.mutex.RLock()
			currentBlock := s.lastBlock
			s.mutex.RUnlock()

			// Check if we're caught up
			if currentBlock >= headBlock {
				s.setSyncCaughtUp(true)
				continue
			}

			// Calculate blocks to process
			blocksToProcess := headBlock - currentBlock
			if blocksToProcess <= 0 {
				s.setSyncCaughtUp(true)
				continue
			}

			// Limit batch size
			batchSize := int64(s.config.Sync.BlockBatchSize)
			if blocksToProcess > batchSize {
				blocksToProcess = batchSize
			}

			// Fetch blocks in batch
			startBlock := currentBlock + 1
			endBlock := currentBlock + blocksToProcess

			blocks, err := s.steem.GetBlocksRange(ctx, startBlock, endBlock)
			if err != nil {
				s.logger.Error("Error fetching blocks range", utils.Error(err))
				s.incrementErrorCount()
				continue
			}

			// Process blocks sequentially
			for _, block := range blocks {
				if err := s.processBlockSequentially(ctx, block); err != nil {
					s.logger.Error("Error processing block",
						utils.Int64("block", block.Number),
						utils.Error(err),
					)
					s.incrementErrorCount()
					continue
				}

				s.updateLastBlock(block.Number)
				s.incrementBlockCount()

				// Update metrics
				utils.BlocksProcessed.WithLabelValues("block_sync").Inc()
				utils.CurrentBlock.WithLabelValues("block_sync").Set(float64(block.Number))
			}

			// Flush buffers
			s.flushBuffers(ctx)
		}
	}
}

// IsSyncCaughtUp returns whether sync has caught up with the latest block
func (s *BlockSyncService) IsSyncCaughtUp() bool {
	s.syncMutex.RLock()
	defer s.syncMutex.RUnlock()
	return s.syncCaughtUp
}

// setSyncCaughtUp sets the sync caught up status
func (s *BlockSyncService) setSyncCaughtUp(caughtUp bool) {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()
	if !s.syncCaughtUp && caughtUp {
		s.logger.Info("Block sync caught up with latest block")
	}
	s.syncCaughtUp = caughtUp
}

// processBlockSequentially processes a single block sequentially (serial processing of operations)
func (s *BlockSyncService) processBlockSequentially(ctx context.Context, block *steem.Block) error {
	// Add block to buffer for batch saving
	s.bufferMutex.Lock()
	s.blockBuffer = append(s.blockBuffer, block)
	shouldFlushBlocks := len(s.blockBuffer) >= s.config.Sync.BlockBatchSize
	bufferCopy := make([]*steem.Block, len(s.blockBuffer))
	copy(bufferCopy, s.blockBuffer)
	if shouldFlushBlocks {
		s.blockBuffer = s.blockBuffer[:0]
	}
	s.bufferMutex.Unlock()

	// Flush block buffer if needed
	if shouldFlushBlocks {
		if err := s.saveBlocksBatch(ctx, bufferCopy); err != nil {
			s.logger.Error("Error saving blocks batch", utils.Error(err))
			// Continue processing even if batch save fails
		}
	}

	// Get operations in block
	ops, err := s.steem.GetOpsInBlock(ctx, block.Number, true)
	if err != nil {
		return fmt.Errorf("failed to get ops in block %d: %w", block.Number, err)
	}

	// Process operations sequentially (in order)
	for _, op := range ops {
		operation := &blockchain.Operation{
			Block:     block,
			Operation: &op,
		}

		// Process operation synchronously (serial processing)
		if err := s.operationProcessor.Process(operation); err != nil {
			s.logger.Error("Error processing operation",
				utils.String("type", op.Op[0].(string)),
				utils.Int64("block", block.Number),
				utils.Error(err),
			)
			s.incrementErrorCount()
			// Continue processing other operations even if one fails
			continue
		}

		s.incrementOperationCount()

		// Update metrics
		if len(op.Op) > 0 {
			if opType, ok := op.Op[0].(string); ok {
				utils.OperationsProcessed.WithLabelValues(opType).Inc()
			}
		}
	}

	return nil
}

// saveBlocksBatch saves multiple blocks to the database using bulk write
func (s *BlockSyncService) saveBlocksBatch(ctx context.Context, blocks []*steem.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		utils.BatchWriteDuration.WithLabelValues("block_sync", "blocks").Observe(duration)
	}()

	// Convert blocks to database models
	operations := make([]mongo.WriteModel, 0, len(blocks))
	for _, block := range blocks {
		// Calculate date index
		dateIndex := block.Timestamp.Format("2006-01-02")

		// Count operations and transactions
		txCount := len(block.Transactions)
		opCount := 0
		transferCount := 0
		voteCount := 0
		commentCount := 0

		// Convert transactions and count operations
		transactions := make([]map[string]interface{}, 0, txCount)
		for _, tx := range block.Transactions {
			txMap := map[string]interface{}{
				"ref_block_num":    tx.RefBlockNum,
				"ref_block_prefix": tx.RefBlockPrefix,
				"expiration":       tx.Expiration,
				"operations":       tx.Operations,
				"extensions":       tx.Extensions,
				"signatures":       tx.Signatures,
				"transaction_id":   tx.TransactionID, // Add transaction ID for tx_id query
			}
			transactions = append(transactions, txMap)
			opCount += len(tx.Operations)
		}

		dbBlock := &database.Block{
			ID:               block.Number,
			Number:           block.Number,
			Hash:             block.BlockID, // Add block hash (block_id)
			Timestamp:        block.Timestamp,
			Previous:         block.Previous,
			Witness:          block.Witness,
			TransactionCount: txCount,
			OperationCount:   opCount,
			Transactions:     transactions,
			DateIndex:        dateIndex,
			TransferCount:    transferCount,
			VoteCount:        voteCount,
			CommentCount:     commentCount,
		}

		// Create upsert operation
		filter := map[string]interface{}{"_id": dbBlock.ID}
		update := map[string]interface{}{"$set": dbBlock}
		op := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)
		operations = append(operations, op)
	}

	// Execute bulk write
	collection := s.db.Collection("blocks")
	opts := options.BulkWrite().SetOrdered(false)
	_, err := collection.BulkWrite(ctx, operations, opts)
	if err != nil {
		return fmt.Errorf("failed to bulk write blocks: %w", err)
	}

	return nil
}

// batchWriter periodically flushes buffers
func (s *BlockSyncService) batchWriter(ctx context.Context) {
	ticker := time.NewTicker(s.config.Sync.BatchWriteInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining buffers on shutdown
			s.flushBuffers(ctx)
			return
		case <-ticker.C:
			s.flushBuffers(ctx)
		}
	}
}

// flushBuffers flushes block buffer
func (s *BlockSyncService) flushBuffers(ctx context.Context) {
	s.bufferMutex.Lock()
	blockBuffer := make([]*steem.Block, len(s.blockBuffer))
	copy(blockBuffer, s.blockBuffer)
	s.blockBuffer = s.blockBuffer[:0]
	s.bufferMutex.Unlock()

	// Flush blocks
	if len(blockBuffer) > 0 {
		if err := s.saveBlocksBatch(ctx, blockBuffer); err != nil {
			s.logger.Error("Error flushing block buffer", utils.Error(err))
		}
	}
}

// flushAccountOperationsBuffer periodically flushes account operations buffer
func (s *BlockSyncService) flushAccountOperationsBuffer(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second) // Flush every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining buffer on shutdown
			if s.operationProcessor != nil {
				if err := s.operationProcessor.FlushAccountOperationsBuffer(ctx); err != nil {
					s.logger.Error("Error flushing account operations buffer on shutdown", utils.Error(err))
				}
			}
			return
		case <-ticker.C:
			if s.operationProcessor != nil {
				if err := s.operationProcessor.FlushAccountOperationsBuffer(ctx); err != nil {
					s.logger.Error("Error flushing account operations buffer", utils.Error(err))
				}
			}
		}
	}
}

// statsReporter reports statistics periodically
func (s *BlockSyncService) statsReporter(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reportStats()
		}
	}
}

// reportStats reports current statistics
func (s *BlockSyncService) reportStats() {
	s.stats.mutex.RLock()
	stats := *s.stats
	s.stats.mutex.RUnlock()

	s.mutex.RLock()
	lastBlock := s.lastBlock
	s.mutex.RUnlock()

	duration := time.Since(stats.StartTime)
	blocksPerSecond := float64(stats.BlocksProcessed) / duration.Seconds()
	opsPerSecond := float64(stats.OperationsProcessed) / duration.Seconds()

	s.logger.Info("Block sync statistics",
		utils.Int64("blocks_processed", stats.BlocksProcessed),
		utils.Int64("operations_processed", stats.OperationsProcessed),
		utils.Int64("errors", stats.ErrorCount),
		utils.Float64("blocks_per_second", blocksPerSecond),
		utils.Float64("ops_per_second", opsPerSecond),
		utils.Int64("last_block", lastBlock),
	)
}

// updateLastBlock updates the last processed block
func (s *BlockSyncService) updateLastBlock(blockNum int64) {
	s.mutex.Lock()
	if blockNum > s.lastBlock {
		s.lastBlock = blockNum
	}
	s.mutex.Unlock()

	// Save to database periodically
	if blockNum%100 == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.db.SaveLastProcessedBlock(ctx, blockNum); err != nil {
			s.logger.Error("Failed to save last processed block", utils.Error(err))
		}
	}
}

// incrementBlockCount increments the block count
func (s *BlockSyncService) incrementBlockCount() {
	s.stats.mutex.Lock()
	s.stats.BlocksProcessed++
	s.stats.LastBlockTime = time.Now()
	s.stats.mutex.Unlock()
}

// incrementOperationCount increments the operation count
func (s *BlockSyncService) incrementOperationCount() {
	s.stats.mutex.Lock()
	s.stats.OperationsProcessed++
	s.stats.mutex.Unlock()
}

// incrementErrorCount increments the error count
func (s *BlockSyncService) incrementErrorCount() {
	s.stats.mutex.Lock()
	s.stats.ErrorCount++
	s.stats.mutex.Unlock()
}
