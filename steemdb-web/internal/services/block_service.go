package services

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemdb/web/internal/database"
	"github.com/steemdb/web/internal/models"
	"github.com/steemdb/web/pkg/utils"
)

// BlockService handles block-related operations
type BlockService struct {
	db     *database.MongoDB
	redis  *database.Redis
	logger utils.Logger
}

// NewBlockService creates a new block service
func NewBlockService(db *database.MongoDB, redis *database.Redis, logger utils.Logger) *BlockService {
	return &BlockService{
		db:     db,
		redis:  redis,
		logger: logger,
	}
}

// GetBlock retrieves a block by number
func (s *BlockService) GetBlock(ctx context.Context, blockNum int64) (*models.Block, error) {
	collection := s.db.Collection("block")
	var block models.Block

	err := collection.FindOne(ctx, bson.M{"number": blockNum}).Decode(&block)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("block not found: %d", blockNum)
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return &block, nil
}

// GetBlocks retrieves multiple blocks with pagination
func (s *BlockService) GetBlocks(ctx context.Context, params models.PaginationParams, sortParams models.SortParams) (*models.BlockSearchResult, error) {
	collection := s.db.Collection("block")

	// Build sort options
	sortField := "number"
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
			Number:           block.Number,
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
	collection := s.db.Collection("block")

	findOptions := options.Find().
		SetSort(bson.M{"number": -1}).
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
			Number:           block.Number,
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
	collection := s.db.Collection("block")

	filter := bson.M{"witness": witness}

	// Count total documents
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocks by witness: %w", err)
	}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{"number": -1}).
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
			Number:           block.Number,
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
	collection := s.db.Collection("operation")

	filter := bson.M{"block_num": blockNum}

	// Calculate pagination
	skip := (params.Page - 1) * params.PageSize
	findOptions := options.Find().
		SetSort(bson.M{"op_num": 1}).
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
	collection := s.db.Collection("block")

	// Get latest block
	var latestBlock models.Block
	err := collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"number": -1})).Decode(&latestBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %w", err)
	}

	// Get total transactions and operations (placeholder values)
	totalTransactions := int64(0)
	totalOperations := int64(0)

	return &models.BlockStats{
		LatestBlockNum:      latestBlock.Number,
		LastIrreversibleNum: latestBlock.Number - 20, // Approximate
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
