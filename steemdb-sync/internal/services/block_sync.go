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

	// Block cache (maps block number to block)
	blockCache      map[int64]*steem.Block
	blockCacheMutex sync.RWMutex

	// Batch buffers
	blockBuffer      []*steem.Block
	operationBuffer  []*blockchain.Operation
	bufferMutex      sync.Mutex

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
		config:          config,
		db:              db,
		steem:           steemClient,
		logger:          logger,
		blockQueue:      make(chan int64, config.Sync.QueueSize),
		opQueue:         make(chan *blockchain.Operation, config.Sync.QueueSize*10),
		blockCache:      make(map[int64]*steem.Block),
		blockBuffer:     make([]*steem.Block, 0, config.Sync.BlockBatchSize),
		operationBuffer: make([]*blockchain.Operation, 0, config.Sync.OperationBatchSize),
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

	// Start batch writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.batchWriter(ctx)
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

// fetchNextBlocks fetches the next batch of blocks using batch API
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

	// Limit batch size to configured block batch size
	batchSize := int64(s.config.Sync.BlockBatchSize)
	if blocksToProcess > batchSize {
		blocksToProcess = batchSize
	}

	// Use batch API to fetch multiple blocks at once
	startBlock := currentBlock + 1
	endBlock := currentBlock + blocksToProcess + 1

	startTime := time.Now()
	blocks, err := s.steem.GetBlocksRange(ctx, startBlock, endBlock)
	duration := time.Since(startTime).Seconds()
	utils.BatchFetchDuration.WithLabelValues("block_sync").Observe(duration)
	
	if err != nil {
		return fmt.Errorf("failed to get blocks range [%d, %d): %w", startBlock, endBlock, err)
	}

	// Cache blocks and queue for processing
	s.blockCacheMutex.Lock()
	for _, block := range blocks {
		s.blockCache[block.Number] = block
	}
	s.blockCacheMutex.Unlock()

	// Queue blocks for processing
	for _, block := range blocks {
		select {
		case s.blockQueue <- block.Number:
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
	// Try to get block from cache first
	s.blockCacheMutex.RLock()
	block, found := s.blockCache[blockNum]
	s.blockCacheMutex.RUnlock()

	// If not in cache, fetch individually (fallback)
	if !found {
		var err error
		block, err = s.steem.GetBlock(ctx, blockNum)
		if err != nil {
			return fmt.Errorf("failed to get block %d: %w", blockNum, err)
		}
	} else {
		// Remove from cache after use to free memory
		s.blockCacheMutex.Lock()
		delete(s.blockCache, blockNum)
		s.blockCacheMutex.Unlock()
	}

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
	ops, err := s.steem.GetOpsInBlock(ctx, blockNum, true)
	if err != nil {
		return fmt.Errorf("failed to get ops in block %d: %w", blockNum, err)
	}

	// Add operations to buffer
	s.bufferMutex.Lock()
	for _, op := range ops {
		operation := &blockchain.Operation{
			Block:     block,
			Operation: &op,
		}
		s.operationBuffer = append(s.operationBuffer, operation)
	}
	shouldFlushOps := len(s.operationBuffer) >= s.config.Sync.OperationBatchSize
	opsBufferCopy := make([]*blockchain.Operation, len(s.operationBuffer))
	copy(opsBufferCopy, s.operationBuffer)
	if shouldFlushOps {
		s.operationBuffer = s.operationBuffer[:0]
	}
	s.bufferMutex.Unlock()

	// Queue operations for processing (or process in batch if buffer is full)
	if shouldFlushOps {
		// Process operations in batch
		for _, op := range opsBufferCopy {
			select {
			case s.opQueue <- op:
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Queue is full, process synchronously
				if err := processor.Process(op); err != nil {
					s.logger.Error("Error processing operation",
						utils.String("type", op.Operation.Op[0].(string)),
						utils.Int64("block", op.Block.Number),
						utils.Error(err),
					)
				}
			}
		}
	} else {
		// Queue operations individually
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

// saveBlock saves a block to the database (kept for backward compatibility)
func (s *BlockSyncService) saveBlock(ctx context.Context, block *steem.Block) error {
	return s.saveBlocksBatch(ctx, []*steem.Block{block})
}

// saveBlocksBatch saves multiple blocks to the database using bulk write
func (s *BlockSyncService) saveBlocksBatch(ctx context.Context, blocks []*steem.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		utils.BatchWriteDuration.WithLabelValues("block_sync", "block_30d").Observe(duration)
	}()

	// Convert blocks to database models
	operations := make([]mongo.WriteModel, 0, len(blocks))
	for _, block := range blocks {
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
	collection := s.db.Collection("block_30d")
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

// flushBuffers flushes block and operation buffers
func (s *BlockSyncService) flushBuffers(ctx context.Context) {
	s.bufferMutex.Lock()
	blockBuffer := make([]*steem.Block, len(s.blockBuffer))
	copy(blockBuffer, s.blockBuffer)
	s.blockBuffer = s.blockBuffer[:0]
	operationBuffer := make([]*blockchain.Operation, len(s.operationBuffer))
	copy(operationBuffer, s.operationBuffer)
	s.operationBuffer = s.operationBuffer[:0]
	s.bufferMutex.Unlock()

	// Flush blocks
	if len(blockBuffer) > 0 {
		if err := s.saveBlocksBatch(ctx, blockBuffer); err != nil {
			s.logger.Error("Error flushing block buffer", utils.Error(err))
		}
	}

	// Flush operations (queue them for processing)
	if len(operationBuffer) > 0 {
		utils.OperationsBatched.WithLabelValues("block_sync").Add(float64(len(operationBuffer)))
		processor := blockchain.NewOperationProcessor(s.db, s.logger)
		for _, op := range operationBuffer {
			select {
			case s.opQueue <- op:
			case <-ctx.Done():
				return
			default:
				// Queue is full, process synchronously
				if err := processor.Process(op); err != nil {
					s.logger.Error("Error processing operation in flush",
						utils.String("type", op.Operation.Op[0].(string)),
						utils.Int64("block", op.Block.Number),
						utils.Error(err),
					)
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
