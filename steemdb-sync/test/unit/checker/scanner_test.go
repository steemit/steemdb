package checker_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/checker"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
)

// getTestMongoURI returns MongoDB URI for testing
func getTestMongoURI() string {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		username := os.Getenv("MONGO_USERNAME")
		password := os.Getenv("MONGO_PASSWORD")
		if username == "" {
			username = "admin"
		}
		if password == "" {
			password = "123456"
		}
		uri = "mongodb://" + username + ":" + password + "@127.0.0.1:27017/steemdb_test?authSource=admin"
	}
	return uri
}

// setupTestScanner creates a test scanner with MongoDB client
func setupTestScanner(t *testing.T) (*checker.Scanner, *mongo.Client) {
	uri := getTestMongoURI()
	cfg := &config.Config{
		Mongo: config.MongoConfig{
			URI:         uri,
			Database:   "steemdb_test",
			MinPoolSize: 1,
			MaxPoolSize: 10,
		},
	}

	client, err := mongo.NewClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return nil, nil
	}

	scanner := checker.NewScanner(client)
	return scanner, client
}

// cleanupTestData cleans up test data by using unique block numbers per test
// Since we can't easily access internal collections, we use unique block numbers
// to avoid conflicts between tests
func cleanupTestData(t *testing.T, client *mongo.Client) {
	// For now, we rely on using unique block numbers in each test
	// to avoid conflicts. In production, you might want to add a Cleanup method.
	_ = t
	_ = client
}

// TestNewScanner tests scanner initialization
func TestNewScanner(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	assert.NotNil(t, scanner)
}

// TestScanCompleteRange tests scanning a complete range of blocks
func TestScanCompleteRange(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	ctx := context.Background()

	// Use unique block numbers to avoid conflicts
	startBlock := uint32(30001)
	testBlocks := []*model.Block{
		{ID: startBlock, BlockNum: startBlock, BlockID: "block30001"},
		{ID: startBlock + 1, BlockNum: startBlock + 1, BlockID: "block30002"},
		{ID: startBlock + 2, BlockNum: startBlock + 2, BlockID: "block30003"},
		{ID: startBlock + 4, BlockNum: startBlock + 4, BlockID: "block30005"}, // Missing block startBlock+3
	}

	err := mongoClient.BulkUpsertBlocks(ctx, testBlocks)
	require.NoError(t, err)

	// Insert operations for all test blocks
	for _, block := range testBlocks {
		op := &model.Operation{
			ID:       model.OperationID(block.BlockNum, 0, 0),
			BlockNum: block.BlockNum,
			TrxID:    "tx1",
			TrxIndex: 0,
			OpIndex:  0,
			OpType:   "transfer",
			OpValue:  map[string]interface{}{"from": "alice"},
			Virtual:  false,
			Source:   "plugin",
		}
		err := mongoClient.BulkUpsertOperations(ctx, []*model.Operation{op})
		require.NoError(t, err)
	}

	// Scan only our test range
	endBlock := startBlock + 4
	result, err := scanner.Scan(ctx, endBlock)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify scan results
	assert.Equal(t, endBlock, result.MaxBlock)
	
	// Should find the missing block (startBlock+3)
	foundMissing := false
	for _, missing := range result.MissingBlocks {
		if missing.BlockNum == startBlock+3 {
			assert.Equal(t, "missing", missing.Reason)
			foundMissing = true
			break
		}
	}
	assert.True(t, foundMissing, "Should find missing block")
}

// TestScanMissingOperations tests scanning for blocks with no operations
func TestScanMissingOperations(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	ctx := context.Background()

	// Use unique block numbers to avoid conflicts with other tests
	blockNum10 := uint32(10010)
	blockNum11 := uint32(10011)

	// Insert blocks without operations
	testBlocks := []*model.Block{
		{ID: blockNum10, BlockNum: blockNum10, BlockID: "block10010"},
		{ID: blockNum11, BlockNum: blockNum11, BlockID: "block10011"},
	}

	err := mongoClient.BulkUpsertBlocks(ctx, testBlocks)
	require.NoError(t, err)

	// Block 10 has operations, block 11 doesn't
	op := &model.Operation{
		ID:       model.OperationID(blockNum10, 0, 0),
		BlockNum: blockNum10,
		TrxID:    "tx1",
		TrxIndex: 0,
		OpIndex:  0,
		OpType:   "transfer",
		OpValue:  map[string]interface{}{"from": "alice"},
		Virtual:  false,
		Source:   "plugin",
	}
	err = mongoClient.BulkUpsertOperations(ctx, []*model.Operation{op})
	require.NoError(t, err)

	// Scan only blocks 10010-10011
	result, err := scanner.Scan(ctx, blockNum11)
	require.NoError(t, err)

	// Should find block 11 as having no operations
	// Note: We need to filter results to only our test blocks
	foundBlock11 := false
	for _, missing := range result.MissingBlocks {
		if missing.BlockNum == blockNum11 {
			assert.Equal(t, "no_operations", missing.Reason)
			foundBlock11 = true
			break
		}
	}
	assert.True(t, foundBlock11, "Should find block 10011 as no_operations")
}

// TestScanEmptyDatabase tests scanning an empty database
// Note: This test may fail if database has existing data from other tests
// We use higher block numbers to minimize conflicts
func TestScanEmptyDatabase(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	ctx := context.Background()

	// Use high block numbers to avoid conflicts
	startBlock := uint32(20000)
	endBlock := uint32(20004)

	// Scan blocks in range (assuming they don't exist)
	result, err := scanner.Scan(ctx, endBlock)
	require.NoError(t, err)

	// All blocks in range should be missing
	// Note: We need to filter to only our test range
	missingInRange := 0
	for _, missing := range result.MissingBlocks {
		if missing.BlockNum >= startBlock && missing.BlockNum <= endBlock {
			assert.Equal(t, "missing", missing.Reason)
			missingInRange++
		}
	}
	
	// Verify we found all blocks in range as missing
	assert.Equal(t, int(endBlock-startBlock+1), missingInRange, 
		"All blocks in range should be missing")
}

// TestMergeRangesConsecutive tests merging consecutive blocks
func TestMergeRangesConsecutive(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	// Create missing blocks: 1, 2, 3, 4, 5 (consecutive)
	missingBlocks := []checker.MissingBlock{
		{BlockNum: 1, Reason: "missing"},
		{BlockNum: 2, Reason: "missing"},
		{BlockNum: 3, Reason: "missing"},
		{BlockNum: 4, Reason: "missing"},
		{BlockNum: 5, Reason: "missing"},
	}

	ranges := scanner.MergeRanges(missingBlocks)

	// Should merge into one range: 1-5
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, uint32(1), ranges[0].Start)
	assert.Equal(t, uint32(5), ranges[0].End)
	assert.Equal(t, uint32(5), ranges[0].Count())
}

// TestMergeRangesNonConsecutive tests merging non-consecutive blocks
func TestMergeRangesNonConsecutive(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	// Create missing blocks: 1, 2, 5, 6, 10 (non-consecutive)
	missingBlocks := []checker.MissingBlock{
		{BlockNum: 1, Reason: "missing"},
		{BlockNum: 2, Reason: "missing"},
		{BlockNum: 5, Reason: "missing"},
		{BlockNum: 6, Reason: "missing"},
		{BlockNum: 10, Reason: "missing"},
	}

	ranges := scanner.MergeRanges(missingBlocks)

	// Should create 3 ranges: 1-2, 5-6, 10
	assert.Equal(t, 3, len(ranges))
	assert.Equal(t, uint32(1), ranges[0].Start)
	assert.Equal(t, uint32(2), ranges[0].End)
	assert.Equal(t, uint32(5), ranges[1].Start)
	assert.Equal(t, uint32(6), ranges[1].End)
	assert.Equal(t, uint32(10), ranges[2].Start)
	assert.Equal(t, uint32(10), ranges[2].End)
}

// TestMergeRangesEmpty tests merging empty list
func TestMergeRangesEmpty(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	ranges := scanner.MergeRanges([]checker.MissingBlock{})
	assert.Nil(t, ranges)
}

// TestMergeRangesSingle tests merging single block
func TestMergeRangesSingle(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	missingBlocks := []checker.MissingBlock{
		{BlockNum: 100, Reason: "missing"},
	}

	ranges := scanner.MergeRanges(missingBlocks)

	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, uint32(100), ranges[0].Start)
	assert.Equal(t, uint32(100), ranges[0].End)
	assert.Equal(t, uint32(1), ranges[0].Count())
}

// TestBlockRangeCount tests BlockRange.Count()
func TestBlockRangeCount(t *testing.T) {
	// Single block range
	range1 := checker.BlockRange{Start: 10, End: 10}
	assert.Equal(t, uint32(1), range1.Count())

	// Multi-block range
	range2 := checker.BlockRange{Start: 10, End: 20}
	assert.Equal(t, uint32(11), range2.Count())

	// Large range
	range3 := checker.BlockRange{Start: 1, End: 1000}
	assert.Equal(t, uint32(1000), range3.Count())
}

// TestBlockRangeString tests BlockRange.String()
func TestBlockRangeString(t *testing.T) {
	// Single block
	range1 := checker.BlockRange{Start: 10, End: 10}
	assert.Equal(t, "10", range1.String())

	// Multi-block range
	range2 := checker.BlockRange{Start: 10, End: 20}
	assert.Equal(t, "10-20", range2.String())

	// Large range
	range3 := checker.BlockRange{Start: 1, End: 1000}
	assert.Equal(t, "1-1000", range3.String())
}

// TestScanMixedMissingAndNoOps tests scanning with both missing blocks and blocks with no operations
func TestScanMixedMissingAndNoOps(t *testing.T) {
	scanner, mongoClient := setupTestScanner(t)
	if scanner == nil {
		return
	}
	defer mongoClient.Close(context.Background())

	ctx := context.Background()

	// Use unique block numbers
	blockNum10 := uint32(40010)
	blockNum11 := uint32(40011)
	blockNum12 := uint32(40012)

	// Insert block 10 (with operations) and block 12 (without operations)
	// Block 11 is missing
	testBlocks := []*model.Block{
		{ID: blockNum10, BlockNum: blockNum10, BlockID: "block40010"},
		{ID: blockNum12, BlockNum: blockNum12, BlockID: "block40012"},
	}

	err := mongoClient.BulkUpsertBlocks(ctx, testBlocks)
	require.NoError(t, err)

	// Block 10 has operations
	op := &model.Operation{
		ID:       model.OperationID(blockNum10, 0, 0),
		BlockNum: blockNum10,
		TrxID:    "tx1",
		TrxIndex: 0,
		OpIndex:  0,
		OpType:   "transfer",
		OpValue:  map[string]interface{}{"from": "alice"},
		Virtual:  false,
		Source:   "plugin",
	}
	err = mongoClient.BulkUpsertOperations(ctx, []*model.Operation{op})
	require.NoError(t, err)

	// Scan only our test range
	result, err := scanner.Scan(ctx, blockNum12)
	require.NoError(t, err)

	// Should find block 11 as missing and block 12 as no_operations
	// Filter to only our test blocks
	found11 := false
	found12 := false
	for _, missing := range result.MissingBlocks {
		if missing.BlockNum == blockNum11 {
			assert.Equal(t, "missing", missing.Reason)
			found11 = true
		}
		if missing.BlockNum == blockNum12 {
			assert.Equal(t, "no_operations", missing.Reason)
			found12 = true
		}
	}
	assert.True(t, found11, "Should find block 40011 as missing")
	assert.True(t, found12, "Should find block 40012 as no_operations")
}
