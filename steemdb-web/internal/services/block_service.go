package services

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// BlockService handles block-related operations
type BlockService struct {
	db          *database.MongoDB
	redis       *database.Redis
	steemClient *steem.Client
	logger      utils.Logger
}

// NewBlockService creates a new block service
func NewBlockService(db *database.MongoDB, redis *database.Redis, steemClient *steem.Client, logger utils.Logger) *BlockService {
	return &BlockService{
		db:          db,
		redis:       redis,
		steemClient: steemClient,
		logger:      logger,
	}
}

// GetBlock retrieves a block by number. The local blocks collection does not
// store transaction details (the sync batcher only keeps block headers), so
// after a local hit the transaction list is enriched from the steem RPC.
// When the RPC call fails the block is still returned with local fields only.
func (s *BlockService) GetBlock(ctx context.Context, blockNum int64) (*models.Block, error) {
	collection := s.db.Collection("blocks")
	var block models.Block

	err := collection.FindOne(ctx, bson.M{"_id": blockNum}).Decode(&block)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("block not found: %d", blockNum)
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	s.enrichTransactions(&block, blockNum)

	return &block, nil
}

// enrichTransactions fills Transactions (and TransactionCount when missing)
// from the steem RPC block. Failures are logged and ignored — the block is
// still useful without the transaction list.
func (s *BlockService) enrichTransactions(block *models.Block, blockNum int64) {
	if len(block.Transactions) > 0 {
		return
	}

	rpcBlock, err := s.steemClient.GetBlock(blockNum)
	if err != nil || rpcBlock == nil {
		s.logger.Warn("Failed to enrich block transactions from RPC",
			utils.Int64("block_num", blockNum), utils.Error(err))
		return
	}

	block.Transactions = make([]models.Transaction, 0, len(rpcBlock.Transactions))
	for i := range rpcBlock.Transactions {
		tx := &rpcBlock.Transactions[i]
		ops := make([]models.Operation, 0, len(tx.Operations))
		for j := range tx.Operations {
			opValue, _ := tx.Operations[j].Value.(map[string]interface{})
			ops = append(ops, models.Operation{
				ID:       fmt.Sprintf("%d:%d:%d", blockNum, i, j),
				BlockNum: uint32(blockNum),
				TrxID:    tx.TransactionID,
				TrxIndex: int32(i),
				OpIndex:  int32(j),
				OpType:   tx.Operations[j].Type,
				OpValue:  opValue,
			})
		}
		block.Transactions = append(block.Transactions, models.Transaction{
			ID:             tx.TransactionID,
			RefBlockNum:    tx.RefBlockNum,
			RefBlockPrefix: tx.RefBlockPrefix,
			Expiration:     tx.Expiration,
			Operations:     ops,
			Extensions:     tx.Extensions,
			Signatures:     tx.Signatures,
			TransactionID:  tx.TransactionID,
		})
	}

	if block.TransactionCount == 0 {
		block.TransactionCount = len(block.Transactions)
	}
	if block.OperationCount == 0 {
		opCount := 0
		for _, tx := range block.Transactions {
			opCount += len(tx.Operations)
		}
		block.OperationCount = opCount
	}
}

// GetBlocks retrieves multiple blocks with pagination
func (s *BlockService) GetBlocks(ctx context.Context, params models.PaginationParams, sortParams models.SortParams) (*models.BlockSearchResult, error) {
	collection := s.db.Collection("blocks")

	// Build sort options
	sortField := "block_num"
	sortOrder := -1
	if sortParams.SortBy != "" {
		sortField = sortParams.SortBy
	}
	if sortParams.SortOrder == "asc" {
		sortOrder = 1
	}

	// Count total documents
	total, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count blocks: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{sortField: sortOrder}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find blocks: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return &models.BlockSearchResult{
		Blocks:   blocks,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetLatestBlocks retrieves the most recent blocks
func (s *BlockService) GetLatestBlocks(ctx context.Context, limit int) ([]models.BlockSummary, error) {
	collection := s.db.Collection("blocks")

	findOptions := options.Find().
		SetSort(bson.M{"block_num": -1}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest blocks: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return blocks, nil
}

// GetBlocksByWitness retrieves blocks produced by a specific witness
func (s *BlockService) GetBlocksByWitness(ctx context.Context, witness string, params models.PaginationParams) (*models.BlockSearchResult, error) {
	collection := s.db.Collection("blocks")

	filter := bson.M{"witness": witness}

	// Count total documents
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocks by witness: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{"block_num": -1}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find blocks by witness: %w", err)
	}
	defer cursor.Close(ctx)

	var blocks []models.BlockSummary
	for cursor.Next(ctx) {
		var block models.Block
		if err := cursor.Decode(&block); err != nil {
			s.logger.Error("Failed to decode block", utils.Error(err))
			continue
		}

		summary := models.BlockSummary{
			Number:           block.BlockNum,
			Timestamp:        block.Timestamp,
			Witness:          block.Witness,
			TransactionCount: block.TransactionCount,
			OperationCount:   block.OperationCount,
			Previous:         block.Previous,
		}
		blocks = append(blocks, summary)
	}

	return &models.BlockSearchResult{
		Blocks:   blocks,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetBlockOperations retrieves operations from a specific block
func (s *BlockService) GetBlockOperations(ctx context.Context, blockNum int64, params models.PaginationParams) ([]models.Operation, error) {
	collection := s.db.Collection("operations")

	filter := bson.M{"block_num": blockNum}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.D{{Key: "trx_index", Value: 1}, {Key: "op_index", Value: 1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(params.PageSize))

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find block operations: %w", err)
	}
	defer cursor.Close(ctx)

	var operations []models.Operation
	for cursor.Next(ctx) {
		var operation models.Operation
		if err := cursor.Decode(&operation); err != nil {
			s.logger.Error("Failed to decode operation", utils.Error(err))
			continue
		}
		operations = append(operations, operation)
	}

	return operations, nil
}

// GetBlockStats retrieves blockchain statistics
func (s *BlockService) GetBlockStats(ctx context.Context) (*models.BlockStats, error) {
	collection := s.db.Collection("blocks")

	// Get latest block
	var latestBlock models.Block
	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"block_num": -1})).Decode(&latestBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %w", err)
	}

	// Get total transactions and operations (placeholder values)
	totalTransactions := int64(0)
	totalOperations := int64(0)

	return &models.BlockStats{
		LatestBlockNum:      int64(latestBlock.BlockNum),
		LastIrreversibleNum: int64(latestBlock.BlockNum) - 20, // Approximate
		HeadBlockTime:       latestBlock.Timestamp,
		TotalTransactions:   totalTransactions,
		TotalOperations:     totalOperations,
		AverageBlockTime:    3.0, // Default 3 seconds
		TransactionsPerHour: 0,   // Placeholder
		OperationsPerHour:   0,   // Placeholder
	}, nil
}

// GetOperationStats retrieves operation type statistics
func (s *BlockService) GetOperationStats(ctx context.Context, timeRange string) ([]models.OperationStats, error) {
	// Return placeholder data for now
	return []models.OperationStats{
		{Type: "comment", Count: 1000, Percentage: 30.0},
		{Type: "vote", Count: 2000, Percentage: 60.0},
		{Type: "transfer", Count: 333, Percentage: 10.0},
	}, nil
}
