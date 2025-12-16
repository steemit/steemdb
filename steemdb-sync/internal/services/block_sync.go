package services

import (
	"context"
	"fmt"
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

	// State (single goroutine, no locks needed)
	lastBlock int64

	// Processor
	operationProcessor *blockchain.OperationProcessor

	// Block buffer for batch writing
	blockBuffer []*steem.Block
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
	}
}

// Start starts the block synchronization service (single goroutine)
func (s *BlockSyncService) Start(ctx context.Context) error {
	s.logger.Info("Starting block sync service (simplified single goroutine)")

	// Load last processed block
	lastBlock, err := s.db.GetLastProcessedBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to load last block: %w", err)
	}
	s.lastBlock = lastBlock
	s.logger.Info("Loaded last processed block", utils.Int64("block", lastBlock))

	// Initialize operation processor
	s.operationProcessor = blockchain.NewOperationProcessor(s.db, s.logger)

	// Single main loop (reference Python version design)
	ticker := time.NewTicker(s.config.Sync.BlockInterval)
	defer ticker.Stop()

	// Track last flush time for account operations buffer
	lastAccountOpFlush := time.Now()
	accountOpFlushInterval := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			// Flush remaining buffers on shutdown
			s.flushBlockBuffer(ctx)
			if s.operationProcessor != nil {
				if err := s.operationProcessor.FlushAccountOperationsBuffer(ctx); err != nil {
					s.logger.Error("Error flushing account operations buffer on shutdown", utils.Error(err))
				}
			}
			return nil
		case <-ticker.C:
			// Get blockchain head
			props, err := s.steem.GetDynamicGlobalProperties(ctx)
			if err != nil {
				s.logger.Error("Error getting dynamic global properties", utils.Error(err))
				continue
			}

			headBlock := props.LastIrreversibleBlockNum

			// Process all blocks until caught up (reference Python version inner loop)
			for s.lastBlock < headBlock {
				// Calculate batch size
				blocksToProcess := headBlock - s.lastBlock
				batchSize := int64(s.config.Sync.BlockBatchSize)
				if blocksToProcess > batchSize {
					blocksToProcess = batchSize
				}

				// Fetch blocks in batch
				startBlock := s.lastBlock + 1
				endBlock := s.lastBlock + blocksToProcess

				blocks, err := s.steem.GetBlocksRange(ctx, startBlock, endBlock)
				if err != nil {
					s.logger.Error("Error fetching blocks range",
						utils.Int64("start", startBlock),
						utils.Int64("end", endBlock),
						utils.Error(err))
					break // Break inner loop, continue outer loop
				}

				// Process blocks sequentially
				for _, block := range blocks {
					if err := s.processBlock(ctx, block); err != nil {
						s.logger.Error("Error processing block",
							utils.Int64("block", block.Number),
							utils.Error(err))
						continue // Continue processing other blocks
					}

					s.lastBlock = block.Number

					// Update metrics
					utils.BlocksProcessed.WithLabelValues("block_sync").Inc()
					utils.CurrentBlock.WithLabelValues("block_sync").Set(float64(block.Number))

					// Save progress periodically (every 100 blocks)
					if s.lastBlock%100 == 0 {
						if err := s.db.SaveLastProcessedBlock(ctx, s.lastBlock); err != nil {
							s.logger.Error("Failed to save last processed block", utils.Error(err))
						}
					}
				}

				// Check if we need to exit
				select {
				case <-ctx.Done():
					return nil
				default:
					// Continue processing next batch
				}
			}

			// Save final progress after processing all blocks
			if err := s.db.SaveLastProcessedBlock(ctx, s.lastBlock); err != nil {
				s.logger.Error("Failed to save last processed block", utils.Error(err))
			}

			// Flush account operations buffer periodically
			if time.Since(lastAccountOpFlush) >= accountOpFlushInterval {
				if s.operationProcessor != nil {
					if err := s.operationProcessor.FlushAccountOperationsBuffer(ctx); err != nil {
						s.logger.Error("Error flushing account operations buffer", utils.Error(err))
					}
				}
				lastAccountOpFlush = time.Now()
			}
		}
	}
}

// processBlock processes a single block
func (s *BlockSyncService) processBlock(ctx context.Context, block *steem.Block) error {
	// Add block to buffer for batch saving
	s.blockBuffer = append(s.blockBuffer, block)

	// Flush block buffer when it reaches batch size
	if len(s.blockBuffer) >= s.config.Sync.BlockBatchSize {
		if err := s.flushBlockBuffer(ctx); err != nil {
			s.logger.Error("Error saving blocks batch", utils.Error(err))
			// Continue processing even if batch save fails
		}
	}

	// Process operations from block transactions (no virtual operations)
	for txIndex, tx := range block.Transactions {
		for opIndex, opData := range tx.Operations {
			// Convert transaction operation to steem.Operation format
			// opData is []interface{} with format [op_type, op_data]
			if len(opData) < 2 {
				continue
			}

			op := &steem.Operation{
				TrxID:      tx.TransactionID,
				Block:      block.Number,
				TrxInBlock: txIndex,
				OpInTrx:    opIndex,
				VirtualOp:  0, // Regular operations, not virtual
				Timestamp:  block.Timestamp,
				Op:         opData,
			}

			operation := &blockchain.Operation{
				Block:     block,
				Operation: op,
			}

			if err := s.operationProcessor.Process(operation); err != nil {
				opType := "unknown"
				if len(opData) > 0 {
					if t, ok := opData[0].(string); ok {
						opType = t
					}
				}
				s.logger.Error("Error processing operation",
					utils.String("type", opType),
					utils.Int64("block", block.Number),
					utils.Error(err))
				continue // Continue processing other operations
			}

			// Update metrics
			if len(opData) > 0 {
				if opType, ok := opData[0].(string); ok {
					utils.OperationsProcessed.WithLabelValues(opType).Inc()
				}
			}
		}
	}

	return nil
}

// flushBlockBuffer flushes the block buffer to database
func (s *BlockSyncService) flushBlockBuffer(ctx context.Context) error {
	if len(s.blockBuffer) == 0 {
		return nil
	}

	blocks := s.blockBuffer
	s.blockBuffer = s.blockBuffer[:0] // Clear buffer

	return s.saveBlocksBatch(ctx, blocks)
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
				"transaction_id":   tx.TransactionID,
			}
			transactions = append(transactions, txMap)
			opCount += len(tx.Operations)
		}

		dbBlock := &database.Block{
			ID:               block.Number,
			Number:           block.Number,
			Hash:             block.BlockID,
			Timestamp:        block.Timestamp,
			Previous:         block.Previous,
			Witness:          block.Witness,
			TransactionCount: txCount,
			OperationCount:   opCount,
			Transactions:     transactions,
			DateIndex:        dateIndex,
			TransferCount:    0, // Will be calculated if needed
			VoteCount:        0, // Will be calculated if needed
			CommentCount:     0, // Will be calculated if needed
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

// IsSyncCaughtUp returns whether sync has caught up with the latest block
// This method is kept for compatibility with CronTab service
func (s *BlockSyncService) IsSyncCaughtUp() bool {
	// In simplified version, we check by comparing with current head block
	// This is a simple check, can be enhanced if needed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, err := s.steem.GetDynamicGlobalProperties(ctx)
	if err != nil {
		return false
	}

	headBlock := props.LastIrreversibleBlockNum
	return s.lastBlock >= headBlock
}
