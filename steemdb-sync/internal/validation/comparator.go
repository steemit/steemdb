package validation

import (
	"fmt"
	"reflect"
	"time"

	"github.com/steemit/steemdb/sync/internal/database"
)

// ComparisonResult represents the result of comparing two data structures
type ComparisonResult struct {
	Field       string
	NewValue    interface{}
	LegacyValue interface{}
	Match       bool
}

// BlockComparison compares block data between new and legacy implementations
type BlockComparison struct {
	BlockNum         int64
	TimestampMatch   bool
	WitnessMatch     bool
	PreviousMatch    bool
	TransactionCount int
	LegacyTxCount    int
	OperationCount   int
	LegacyOpCount    int
	Discrepancies    []ComparisonResult
}

// CompareBlocks compares block data from new and legacy databases
func CompareBlocks(newBlock *database.Block, legacyBlock map[string]interface{}) *BlockComparison {
	result := &BlockComparison{
		Discrepancies: make([]ComparisonResult, 0),
	}

	if newBlock == nil || legacyBlock == nil {
		return result
	}

	result.BlockNum = newBlock.Number

	// Compare timestamp
	if legacyTS, ok := legacyBlock["_ts"].(time.Time); ok {
		result.TimestampMatch = newBlock.Timestamp.Equal(legacyTS)
		if !result.TimestampMatch {
			result.Discrepancies = append(result.Discrepancies, ComparisonResult{
				Field:       "timestamp",
				NewValue:    newBlock.Timestamp,
				LegacyValue: legacyTS,
				Match:       false,
			})
		}
	}

	// Compare witness
	if legacyWitness, ok := legacyBlock["witness"].(string); ok {
		result.WitnessMatch = newBlock.Witness == legacyWitness
		if !result.WitnessMatch {
			result.Discrepancies = append(result.Discrepancies, ComparisonResult{
				Field:       "witness",
				NewValue:    newBlock.Witness,
				LegacyValue: legacyWitness,
				Match:       false,
			})
		}
	}

	// Compare previous
	if legacyPrevious, ok := legacyBlock["previous"].(string); ok {
		result.PreviousMatch = newBlock.Previous == legacyPrevious
		if !result.PreviousMatch {
			result.Discrepancies = append(result.Discrepancies, ComparisonResult{
				Field:       "previous",
				NewValue:    newBlock.Previous,
				LegacyValue: legacyPrevious,
				Match:       false,
			})
		}
	}

	// Compare transaction count
	result.TransactionCount = len(newBlock.Transactions)
	if legacyTxs, ok := legacyBlock["transactions"].([]interface{}); ok {
		result.LegacyTxCount = len(legacyTxs)
		if result.TransactionCount != result.LegacyTxCount {
			result.Discrepancies = append(result.Discrepancies, ComparisonResult{
				Field:       "transaction_count",
				NewValue:    result.TransactionCount,
				LegacyValue: result.LegacyTxCount,
				Match:       false,
			})
		}
	}

	return result
}

// CompareOperations compares operation counts and key data
func CompareOperations(newOps map[string]int, legacyOps map[string]int) []ComparisonResult {
	discrepancies := make([]ComparisonResult, 0)

	// Get all operation types
	allTypes := make(map[string]bool)
	for opType := range newOps {
		allTypes[opType] = true
	}
	for opType := range legacyOps {
		allTypes[opType] = true
	}

	// Compare counts for each operation type
	for opType := range allTypes {
		newCount := newOps[opType]
		legacyCount := legacyOps[opType]
		if newCount != legacyCount {
			discrepancies = append(discrepancies, ComparisonResult{
				Field:       fmt.Sprintf("operation_count_%s", opType),
				NewValue:    newCount,
				LegacyValue: legacyCount,
				Match:       false,
			})
		}
	}

	return discrepancies
}

// CompareDocuments compares document counts in collections
func CompareDocuments(newCount int, legacyCount int, collection string) *ComparisonResult {
	if newCount != legacyCount {
		return &ComparisonResult{
			Field:       fmt.Sprintf("document_count_%s", collection),
			NewValue:    newCount,
			LegacyValue: legacyCount,
			Match:       false,
		}
	}
	return nil
}

// DeepCompare compares two values deeply
func DeepCompare(newVal, legacyVal interface{}) bool {
	if newVal == nil && legacyVal == nil {
		return true
	}
	if newVal == nil || legacyVal == nil {
		return false
	}

	// Handle time.Time comparison
	if newTime, ok := newVal.(time.Time); ok {
		if legacyTime, ok := legacyVal.(time.Time); ok {
			return newTime.Equal(legacyTime)
		}
		return false
	}

	// Use reflect for deep comparison
	return reflect.DeepEqual(newVal, legacyVal)
}

// FormatComparisonResult formats a comparison result for logging
func FormatComparisonResult(result *ComparisonResult) string {
	return fmt.Sprintf("Field: %s, New: %v, Legacy: %v, Match: %v",
		result.Field, result.NewValue, result.LegacyValue, result.Match)
}
