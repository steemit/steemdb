package validation

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Validator validates data correctness by comparing with legacy implementation
type Validator struct {
	newDB      *database.MongoDB
	legacyDB   *mongo.Database
	logger     utils.Logger
	sampleRate float64 // Percentage of blocks to validate (0.01 = 1%)
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	BlockNum      int64
	Valid         bool
	Discrepancies []ComparisonResult
	Error         error
}

// ValidationReport represents a summary of validation results
type ValidationReport struct {
	TotalBlocks     int
	ValidatedBlocks int
	ValidBlocks     int
	InvalidBlocks   int
	Discrepancies   []ValidationResult
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
}

// NewValidator creates a new validator instance
func NewValidator(newDB *database.MongoDB, legacyDB *mongo.Database, logger utils.Logger, sampleRate float64) *Validator {
	return &Validator{
		newDB:      newDB,
		legacyDB:   legacyDB,
		logger:     logger,
		sampleRate: sampleRate,
	}
}

// ValidateBlockRange validates blocks in a specific range
func (v *Validator) ValidateBlockRange(ctx context.Context, startBlock, endBlock int64) (*ValidationReport, error) {
	report := &ValidationReport{
		StartTime:     time.Now(),
		Discrepancies: make([]ValidationResult, 0),
	}

	totalBlocks := endBlock - startBlock + 1
	report.TotalBlocks = int(totalBlocks)

	// Sample blocks for validation
	sampleSize := int(float64(totalBlocks) * v.sampleRate)
	if sampleSize < 1 {
		sampleSize = 1
	}

	// Generate random block numbers to validate
	rand.Seed(time.Now().UnixNano())
	sampledBlocks := make(map[int64]bool)
	for len(sampledBlocks) < sampleSize {
		blockNum := startBlock + rand.Int63n(totalBlocks)
		sampledBlocks[blockNum] = true
	}

	report.ValidatedBlocks = len(sampledBlocks)

	// Validate each sampled block
	for blockNum := range sampledBlocks {
		result := v.ValidateBlock(ctx, blockNum)
		report.Discrepancies = append(report.Discrepancies, result)
		if result.Valid {
			report.ValidBlocks++
		} else {
			report.InvalidBlocks++
		}
	}

	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	return report, nil
}

// ValidateBlock validates a single block
func (v *Validator) ValidateBlock(ctx context.Context, blockNum int64) ValidationResult {
	result := ValidationResult{
		BlockNum:      blockNum,
		Discrepancies: make([]ComparisonResult, 0),
	}

	// Get block from new database
	newBlock, err := v.getNewBlock(ctx, blockNum)
	if err != nil {
		result.Error = fmt.Errorf("failed to get new block %d: %w", blockNum, err)
		result.Valid = false
		return result
	}

	// Get block from legacy database
	legacyBlock, err := v.getLegacyBlock(ctx, blockNum)
	if err != nil {
		result.Error = fmt.Errorf("failed to get legacy block %d: %w", blockNum, err)
		result.Valid = false
		return result
	}

	// Compare blocks
	comparison := CompareBlocks(newBlock, legacyBlock)
	result.Discrepancies = comparison.Discrepancies
	result.Valid = len(comparison.Discrepancies) == 0

	// Compare operation counts
	newOps, err := v.getNewOperationCounts(ctx, blockNum)
	if err == nil {
		legacyOps, err := v.getLegacyOperationCounts(ctx, blockNum)
		if err == nil {
			opDiscrepancies := CompareOperations(newOps, legacyOps)
			result.Discrepancies = append(result.Discrepancies, opDiscrepancies...)
			if len(opDiscrepancies) > 0 {
				result.Valid = false
			}
		}
	}

	return result
}

// getNewBlock retrieves a block from the new database
func (v *Validator) getNewBlock(ctx context.Context, blockNum int64) (*database.Block, error) {
	collection := v.newDB.Collection("block_30d")
	var block database.Block
	err := collection.FindOne(ctx, bson.M{"_id": blockNum}).Decode(&block)
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// getLegacyBlock retrieves a block from the legacy database
func (v *Validator) getLegacyBlock(ctx context.Context, blockNum int64) (map[string]interface{}, error) {
	collection := v.legacyDB.Collection("block_30d")
	var block map[string]interface{}
	err := collection.FindOne(ctx, bson.M{"_id": blockNum}).Decode(&block)
	if err != nil {
		return nil, err
	}
	return block, nil
}

// getNewOperationCounts gets operation counts from new database
func (v *Validator) getNewOperationCounts(ctx context.Context, blockNum int64) (map[string]int, error) {
	counts := make(map[string]int)

	// Get counts from various collections
	// Note: New implementation uses "block_num" field
	collections := map[string]string{
		"vote":            "block_num",
		"transfer":        "block_num",
		"comment":         "block_num",
		"author_reward":   "block_num",
		"curation_reward": "block_num",
	}

	for collName, blockField := range collections {
		collection := v.newDB.Collection(collName)
		count, err := collection.CountDocuments(ctx, bson.M{blockField: blockNum})
		if err == nil {
			counts[collName] = int(count)
		}
	}

	return counts, nil
}

// getLegacyOperationCounts gets operation counts from legacy database
func (v *Validator) getLegacyOperationCounts(ctx context.Context, blockNum int64) (map[string]int, error) {
	counts := make(map[string]int)

	// Get counts from various collections
	// Legacy uses "block" field for most operations
	collections := map[string]string{
		"vote":            "block",
		"transfer":        "block",
		"comment":         "block",
		"author_reward":   "block",
		"curation_reward": "block",
	}

	for collName, blockField := range collections {
		collection := v.legacyDB.Collection(collName)
		count, err := collection.CountDocuments(ctx, bson.M{blockField: blockNum})
		if err == nil {
			counts[collName] = int(count)
		}
	}

	return counts, nil
}

// ValidateCollectionCounts compares document counts in collections
func (v *Validator) ValidateCollectionCounts(ctx context.Context) ([]ComparisonResult, error) {
	discrepancies := make([]ComparisonResult, 0)

	collections := []string{"block_30d", "vote", "transfer", "comment", "author_reward", "curation_reward"}

	for _, collName := range collections {
		newColl := v.newDB.Collection(collName)
		legacyColl := v.legacyDB.Collection(collName)

		newCount, err := newColl.CountDocuments(ctx, bson.M{})
		if err != nil {
			continue
		}

		legacyCount, err := legacyColl.CountDocuments(ctx, bson.M{})
		if err != nil {
			continue
		}

		result := CompareDocuments(int(newCount), int(legacyCount), collName)
		if result != nil {
			discrepancies = append(discrepancies, *result)
		}
	}

	return discrepancies, nil
}

// GenerateReport generates a formatted validation report
func (v *Validator) GenerateReport(report *ValidationReport) string {
	reportStr := fmt.Sprintf(`
Validation Report
==================
Total Blocks: %d
Validated Blocks: %d (%.2f%%)
Valid Blocks: %d
Invalid Blocks: %d
Duration: %v

`, report.TotalBlocks, report.ValidatedBlocks,
		float64(report.ValidatedBlocks)/float64(report.TotalBlocks)*100,
		report.ValidBlocks, report.InvalidBlocks, report.Duration)

	if len(report.Discrepancies) > 0 {
		reportStr += "Discrepancies:\n"
		for _, result := range report.Discrepancies {
			if !result.Valid {
				reportStr += fmt.Sprintf("  Block %d: %d discrepancies\n", result.BlockNum, len(result.Discrepancies))
				for _, disc := range result.Discrepancies {
					reportStr += fmt.Sprintf("    - %s\n", FormatComparisonResult(&disc))
				}
			}
		}
	}

	return reportStr
}
