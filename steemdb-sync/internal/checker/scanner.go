package checker

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/mongo"
)

// Scanner scans the database for missing blocks and operations
type Scanner struct {
	mongoClient *mongo.Client
}

// NewScanner creates a new scanner
func NewScanner(mongoClient *mongo.Client) *Scanner {
	return &Scanner{
		mongoClient: mongoClient,
	}
}

// MissingBlock represents a missing block
type MissingBlock struct {
	BlockNum uint32
	Reason   string // "missing" or "no_operations"
}

// ScanResult contains the scan results
type ScanResult struct {
	MaxBlock      uint32
	MissingBlocks []MissingBlock
	TotalScanned  uint32
}

// Scan scans blocks from 1 to maxBlock and identifies missing blocks
func (s *Scanner) Scan(ctx context.Context, maxBlock uint32) (*ScanResult, error) {
	result := &ScanResult{
		MaxBlock:      maxBlock,
		MissingBlocks: make([]MissingBlock, 0),
		TotalScanned:  0,
	}

	// Scan blocks sequentially
	for blockNum := uint32(1); blockNum <= maxBlock; blockNum++ {
		// Check if block exists
		exists, err := s.mongoClient.CheckBlockExists(ctx, blockNum)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check block %d", blockNum)
		}

		if !exists {
			result.MissingBlocks = append(result.MissingBlocks, MissingBlock{
				BlockNum: blockNum,
				Reason:   "missing",
			})
			result.TotalScanned++
			continue
		}

		// Check if block has operations
		ops, err := s.mongoClient.GetOperationsByBlock(ctx, blockNum)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get operations for block %d", blockNum)
		}

		if len(ops) == 0 {
			result.MissingBlocks = append(result.MissingBlocks, MissingBlock{
				BlockNum: blockNum,
				Reason:   "no_operations",
			})
		}

		result.TotalScanned++

		// Log progress every 10000 blocks
		if blockNum%10000 == 0 {
			fmt.Printf("Scanned %d/%d blocks, found %d missing blocks\n",
				blockNum, maxBlock, len(result.MissingBlocks))
		}
	}

	return result, nil
}

// MergeRanges merges consecutive missing blocks into ranges
func (s *Scanner) MergeRanges(missingBlocks []MissingBlock) []BlockRange {
	if len(missingBlocks) == 0 {
		return nil
	}

	ranges := make([]BlockRange, 0)
	currentStart := missingBlocks[0].BlockNum
	currentEnd := missingBlocks[0].BlockNum

	for i := 1; i < len(missingBlocks); i++ {
		if missingBlocks[i].BlockNum == currentEnd+1 {
			// Consecutive, extend range
			currentEnd = missingBlocks[i].BlockNum
		} else {
			// Gap found, save current range and start new one
			ranges = append(ranges, BlockRange{
				Start: currentStart,
				End:   currentEnd,
			})
			currentStart = missingBlocks[i].BlockNum
			currentEnd = missingBlocks[i].BlockNum
		}
	}

	// Add the last range
	ranges = append(ranges, BlockRange{
		Start: currentStart,
		End:   currentEnd,
	})

	return ranges
}

// BlockRange represents a range of missing blocks
type BlockRange struct {
	Start uint32
	End   uint32
}

// Count returns the number of blocks in the range
func (r BlockRange) Count() uint32 {
	return r.End - r.Start + 1
}

// String returns a string representation of the range
func (r BlockRange) String() string {
	if r.Start == r.End {
		return fmt.Sprintf("%d", r.Start)
	}
	return fmt.Sprintf("%d-%d", r.Start, r.End)
}
