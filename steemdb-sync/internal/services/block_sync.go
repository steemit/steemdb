package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/steemdb/sync/internal/blockchain"
	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// BlockSyncService handles blockchain synchronization
type BlockSyncService struct {
	config *utils.Config
	db     *database.MongoDB
	steem  *steem.Client
	logger utils.Logger

	// State
	lastBlock int64
	mutex     sync.RWMutex

	// Work queues
	blockQueue chan int64
	opQueue    chan *blockchain.Operation

	// Statistics
	stats *SyncStats
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
		config:     config,
		db:         db,
		steem:      steemClient,
		logger:     logger,
		blockQueue: make(chan int64, config.Sync.QueueSize),
		opQueue:    make(chan *blockchain.Operation, config.Sync.QueueSize*10),
		stats: &SyncStats{
			StartTime: time.Now(),
		},
	}
}

// Start starts the block synchronization service
func (s *BlockSyncService) Start(ctx context.Context) error {
	s.logger.Info("Starting block sync service")

	// Load last processed block
	if err := s.loadLastBlock(ctx); err != nil {
		return fmt.Errorf("failed to load last block: %w", err)
	}

	var wg sync.WaitGroup

	// Start block fetcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.blockFetcher(ctx)
	}()

	// Start block processors
	for i := 0; i < s.config.Sync.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			s.blockProcessor(ctx, workerID)
		}(i)
	}

	// Start operation processors
	for i := 0; i < s.config.Sync.Workers*2; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			s.operationProcessor(ctx, workerID)
		}(i)
	}

	// Start statistics reporter
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.statsReporter(ctx)
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

// blockFetcher fetches blocks from the blockchain
func (s *BlockSyncService) blockFetcher(ctx context.Context) {
	ticker := time.NewTicker(s.config.Sync.BlockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.fetchNextBlocks(ctx); err != nil {
				s.logger.Error("Error fetching blocks", utils.Error(err))
				s.incrementErrorCount()
			}
		}
	}
}

// fetchNextBlocks fetches the next batch of blocks
func (s *BlockSyncService) fetchNextBlocks(ctx context.Context) error {
	// Get blockchain head
	props, err := s.steem.GetDynamicGlobalProperties(ctx)
	if err != nil {
		return fmt.Errorf("failed to get dynamic global properties: %w", err)
	}

	headBlock := props.LastIrreversibleBlockNum

	s.mutex.RLock()
	currentBlock := s.lastBlock
	s.mutex.RUnlock()

	// Calculate blocks to process
	blocksToProcess := headBlock - currentBlock
	if blocksToProcess <= 0 {
		return nil
	}

	// Limit batch size
	if blocksToProcess > int64(s.config.Sync.BatchSize) {
		blocksToProcess = int64(s.config.Sync.BatchSize)
	}

	// Queue blocks for processing
	for i := int64(1); i <= blocksToProcess; i++ {
		blockNum := currentBlock + i
		select {
		case s.blockQueue <- blockNum:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Queue is full, skip this batch
			return nil
		}
	}

	return nil
}

// blockProcessor processes blocks from the queue
func (s *BlockSyncService) blockProcessor(ctx context.Context, workerID int) {
	processor := blockchain.NewOperationProcessor(s.db, s.logger)

	for {
		select {
		case <-ctx.Done():
			return
		case blockNum := <-s.blockQueue:
			if err := s.processBlock(ctx, blockNum, processor); err != nil {
				s.logger.Error("Error processing block",
					utils.Int64("block", blockNum),
					utils.Int("worker", workerID),
					utils.Error(err),
				)
				s.incrementErrorCount()
				utils.ErrorsTotal.WithLabelValues("block_sync", "block_processing").Inc()
				continue
			}

			s.updateLastBlock(blockNum)
			s.incrementBlockCount()
			
			// Update metrics
			utils.BlocksProcessed.WithLabelValues("block_sync").Inc()
			utils.CurrentBlock.WithLabelValues("block_sync").Set(float64(blockNum))
		}
	}
}

// processBlock processes a single block
func (s *BlockSyncService) processBlock(ctx context.Context, blockNum int64, processor *blockchain.OperationProcessor) error {
	// Get block data
	block, err := s.steem.GetBlock(ctx, blockNum)
	if err != nil {
		return fmt.Errorf("failed to get block %d: %w", blockNum, err)
	}

	// Save block to database
	if err := s.saveBlock(ctx, block); err != nil {
		return fmt.Errorf("failed to save block %d: %w", blockNum, err)
	}

	// Get operations in block
	ops, err := s.steem.GetOpsInBlock(ctx, blockNum, true)
	if err != nil {
		return fmt.Errorf("failed to get ops in block %d: %w", blockNum, err)
	}

	// Process operations
	for _, op := range ops {
		operation := &blockchain.Operation{
			Block:     block,
			Operation: &op,
		}

		select {
		case s.opQueue <- operation:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Queue is full, process synchronously
			if err := processor.Process(operation); err != nil {
				s.logger.Error("Error processing operation",
					utils.String("type", op.Op[0].(string)),
					utils.Int64("block", blockNum),
					utils.Error(err),
				)
			}
		}
	}

	return nil
}

// operationProcessor processes operations from the queue
func (s *BlockSyncService) operationProcessor(ctx context.Context, workerID int) {
	processor := blockchain.NewOperationProcessor(s.db, s.logger)

	for {
		select {
		case <-ctx.Done():
			return
		case op := <-s.opQueue:
			if err := processor.Process(op); err != nil {
				s.logger.Error("Error processing operation",
					utils.String("type", op.Operation.Op[0].(string)),
					utils.Int64("block", op.Block.Number),
					utils.Int("worker", workerID),
					utils.Error(err),
				)
				s.incrementErrorCount()
				continue
			}
			s.incrementOperationCount()
			
			// Update metrics
			if len(op.Operation.Op) > 0 {
				if opType, ok := op.Operation.Op[0].(string); ok {
					utils.OperationsProcessed.WithLabelValues(opType).Inc()
				}
			}
		}
	}
}

// saveBlock saves a block to the database
func (s *BlockSyncService) saveBlock(ctx context.Context, block *steem.Block) error {
	// Convert to database model
	dbBlock := &database.Block{
		ID:        block.Number,
		Number:    block.Number,
		Timestamp: block.Timestamp,
		Previous:  block.Previous,
		Witness:   block.Witness,
	}

	// Convert transactions
	for _, tx := range block.Transactions {
		txMap := map[string]interface{}{
			"ref_block_num":    tx.RefBlockNum,
			"ref_block_prefix": tx.RefBlockPrefix,
			"expiration":       tx.Expiration,
			"operations":       tx.Operations,
			"extensions":       tx.Extensions,
			"signatures":       tx.Signatures,
		}
		dbBlock.Transactions = append(dbBlock.Transactions, txMap)
	}

	// Save to database
	collection := s.db.Collection("block_30d")
	_, err := collection.InsertOne(ctx, dbBlock)
	if err != nil {
		return fmt.Errorf("failed to insert block: %w", err)
	}

	return nil
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
