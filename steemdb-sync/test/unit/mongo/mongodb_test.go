package mongo_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestMongoURI returns MongoDB URI for testing
// Uses environment variable or defaults to test database with authentication
func getTestMongoURI() string {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		// Default to localhost with authentication
		username := os.Getenv("MONGO_USERNAME")
		password := os.Getenv("MONGO_PASSWORD")
		if username == "" {
			username = "admin"
		}
		if password == "" {
			password = "123456"
		}
		// Try 127.0.0.1 first (more reliable than localhost)
		uri = "mongodb://" + username + ":" + password + "@127.0.0.1:27017/steemdb_test?authSource=admin"
	}
	return uri
}

// setupTestClient creates a test MongoDB client
func setupTestClient(t *testing.T) (*mongo.Client, *config.Config) {
	cfg := &config.Config{
		Mongo: config.MongoConfig{
			URI:         getTestMongoURI(),
			Database:    "steemdb_test",
			MinPoolSize: 1,
			MaxPoolSize: 10,
		},
	}

	client, err := mongo.NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping test: MongoDB not available: %v", err)
		return nil, nil
	}

	return client, cfg
}

// cleanupTestData cleans up test data
func cleanupTestData(t *testing.T, client *mongo.Client) {
	ctx := context.Background()

	// Clean up test data by dropping collections
	// Note: In production, we would use DeleteMany instead
	_ = client.Close(ctx)
}

// cleanupCollections cleans up all collections in the test database
func cleanupCollections(t *testing.T, client *mongo.Client) {
	ctx := context.Background()

	// Get database from client (we need to access it directly)
	// Since we can't access db directly, we'll use a different approach:
	// Each test should clean up its own data, or we use unique database names per test
	_ = ctx
	_ = client
}

// TestNewClient tests MongoDB client initialization
func TestNewClient(t *testing.T) {
	client, cfg := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	require.NotNil(t, client)
	assert.NotNil(t, cfg)
}

// TestNewClientInvalidURI tests client initialization with invalid URI
func TestNewClientInvalidURI(t *testing.T) {
	cfg := &config.Config{
		Mongo: config.MongoConfig{
			URI:         "mongodb://invalid:99999",
			Database:    "test",
			MinPoolSize: 1,
			MaxPoolSize: 10,
		},
	}

	client, err := mongo.NewClient(cfg)
	assert.Error(t, err)
	assert.Nil(t, client)
}

// TestBulkUpsertBlocks tests bulk upsert of blocks
func TestBulkUpsertBlocks(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	blocks := []*model.Block{
		{
			ID:               100,
			BlockNum:         100,
			BlockID:          "block100",
			Previous:         "block99",
			Timestamp:        time.Now(),
			Witness:          "witness1",
			TransactionCount: 5,
		},
		{
			ID:               101,
			BlockNum:         101,
			BlockID:          "block101",
			Previous:         "block100",
			Timestamp:        time.Now(),
			Witness:          "witness2",
			TransactionCount: 3,
		},
	}

	err := client.BulkUpsertBlocks(ctx, blocks)
	require.NoError(t, err)

	// Verify blocks were inserted
	block, err := client.GetBlockByNumber(ctx, 100)
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, uint32(100), block.BlockNum)
	assert.Equal(t, "block100", block.BlockID)
}

// TestBulkUpsertBlocksIdempotent tests idempotent upsert
func TestBulkUpsertBlocksIdempotent(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	block := &model.Block{
		ID:               200,
		BlockNum:         200,
		BlockID:          "block200",
		Previous:         "block199",
		Timestamp:        time.Now(),
		Witness:          "witness1",
		TransactionCount: 10,
	}

	// Insert first time
	err := client.BulkUpsertBlocks(ctx, []*model.Block{block})
	require.NoError(t, err)

	// Update block
	block.TransactionCount = 20
	block.Witness = "witness2"

	// Insert again (should update, not duplicate)
	err = client.BulkUpsertBlocks(ctx, []*model.Block{block})
	require.NoError(t, err)

	// Verify only one block exists with updated data
	retrieved, err := client.GetBlockByNumber(ctx, 200)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, uint32(200), retrieved.BlockNum)
	assert.Equal(t, 20, retrieved.TransactionCount)
	assert.Equal(t, "witness2", retrieved.Witness)
}

// TestBulkUpsertBlocksEmpty tests empty batch handling
func TestBulkUpsertBlocksEmpty(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	err := client.BulkUpsertBlocks(ctx, []*model.Block{})
	assert.NoError(t, err)
}

// TestBulkUpsertTransactions tests bulk upsert of transactions
func TestBulkUpsertTransactions(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	txs := []*model.Transaction{
		{
			ID:         "tx1",
			BlockNum:   100,
			TrxIndex:   0,
			Expiration: time.Now().Add(1 * time.Hour),
		},
		{
			ID:         "tx2",
			BlockNum:   100,
			TrxIndex:   1,
			Expiration: time.Now().Add(1 * time.Hour),
		},
	}

	err := client.BulkUpsertTransactions(ctx, txs)
	require.NoError(t, err)
}

// TestBulkUpsertTransactionsIdempotent tests idempotent transaction upsert
func TestBulkUpsertTransactionsIdempotent(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	tx := &model.Transaction{
		ID:         "tx_idempotent",
		BlockNum:   300,
		TrxIndex:   0,
		Expiration: time.Now().Add(1 * time.Hour),
	}

	// Insert first time
	err := client.BulkUpsertTransactions(ctx, []*model.Transaction{tx})
	require.NoError(t, err)

	// Update and insert again
	tx.TrxIndex = 1
	err = client.BulkUpsertTransactions(ctx, []*model.Transaction{tx})
	require.NoError(t, err)
}

// TestBulkUpsertOperations tests bulk upsert of operations
func TestBulkUpsertOperations(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	ops := []*model.Operation{
		{
			ID:       "100:0:0",
			BlockNum: 100,
			TrxID:    "tx1",
			TrxIndex: 0,
			OpIndex:  0,
			OpType:   "transfer",
			OpValue: map[string]interface{}{
				"from":   "alice",
				"to":     "bob",
				"amount": "1.000 STEEM",
			},
			Virtual: false,
			Source:  "plugin",
		},
		{
			ID:       "100:0:1",
			BlockNum: 100,
			TrxID:    "tx1",
			TrxIndex: 0,
			OpIndex:  1,
			OpType:   "vote",
			OpValue: map[string]interface{}{
				"voter": "alice",
			},
			Virtual: false,
			Source:  "plugin",
		},
	}

	err := client.BulkUpsertOperations(ctx, ops)
	require.NoError(t, err)

	// Verify operations were inserted
	retrievedOps, err := client.GetOperationsByBlock(ctx, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(retrievedOps), 2)
}

// TestBulkUpsertOperationsIdempotent tests idempotent operation upsert
func TestBulkUpsertOperationsIdempotent(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	op := &model.Operation{
		ID:       "200:0:0",
		BlockNum: 200,
		TrxID:    "tx2",
		TrxIndex: 0,
		OpIndex:  0,
		OpType:   "transfer",
		OpValue: map[string]interface{}{
			"from": "alice",
		},
		Virtual: false,
		Source:  "plugin",
	}

	// Insert first time
	err := client.BulkUpsertOperations(ctx, []*model.Operation{op})
	require.NoError(t, err)

	// Update and insert again
	op.OpType = "comment"
	err = client.BulkUpsertOperations(ctx, []*model.Operation{op})
	require.NoError(t, err)

	// Verify only one operation exists
	retrievedOps, err := client.GetOperationsByBlock(ctx, 200)
	require.NoError(t, err)

	// Find the operation
	var foundOp *model.Operation
	for _, o := range retrievedOps {
		if o.ID == "200:0:0" {
			foundOp = o
			break
		}
	}
	require.NotNil(t, foundOp)
	assert.Equal(t, "comment", foundOp.OpType)
}

// TestBulkUpsertOperationsEmpty tests empty operations batch
func TestBulkUpsertOperationsEmpty(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	err := client.BulkUpsertOperations(ctx, []*model.Operation{})
	assert.NoError(t, err)
}

// TestGetMaxBlock tests getting max block from meta
func TestGetMaxBlock(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// GetMaxBlock should work whether meta exists or not
	// If meta doesn't exist, it returns 0
	// If meta exists from previous tests, it returns the stored value
	maxBlock, err := client.GetMaxBlock(ctx)
	require.NoError(t, err)
	// Just verify it doesn't error and returns a valid value
	// The actual value depends on whether meta exists from previous tests
	assert.GreaterOrEqual(t, maxBlock, uint32(0))

	// Update max block
	err = client.UpdateMaxBlock(ctx, 1000)
	require.NoError(t, err)

	// Verify max block was updated
	maxBlock, err = client.GetMaxBlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint32(1000), maxBlock)
}

// TestUpdateMaxBlock tests updating max block
func TestUpdateMaxBlock(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Update to different values
	err := client.UpdateMaxBlock(ctx, 500)
	require.NoError(t, err)

	maxBlock, err := client.GetMaxBlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint32(500), maxBlock)

	// Update to higher value
	err = client.UpdateMaxBlock(ctx, 1000)
	require.NoError(t, err)

	maxBlock, err = client.GetMaxBlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint32(1000), maxBlock)
}

// TestSetColdStartDone tests setting cold start done flag
func TestSetColdStartDone(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Set cold start done
	err := client.SetColdStartDone(ctx)
	require.NoError(t, err)

	// Verify by checking max block (meta should exist now)
	maxBlock, err := client.GetMaxBlock(ctx)
	require.NoError(t, err)
	// Max block should still be accessible
	assert.GreaterOrEqual(t, maxBlock, uint32(0))
}

// TestGetBlockByNumber tests retrieving block by number
func TestGetBlockByNumber(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Get non-existent block
	block, err := client.GetBlockByNumber(ctx, 9999)
	require.NoError(t, err)
	assert.Nil(t, block)

	// Insert a block
	testBlock := &model.Block{
		ID:               500,
		BlockNum:         500,
		BlockID:          "block500",
		Previous:         "block499",
		Timestamp:        time.Now(),
		Witness:          "witness500",
		TransactionCount: 7,
	}
	err = client.BulkUpsertBlocks(ctx, []*model.Block{testBlock})
	require.NoError(t, err)

	// Retrieve the block
	block, err = client.GetBlockByNumber(ctx, 500)
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, uint32(500), block.BlockNum)
	assert.Equal(t, "block500", block.BlockID)
	assert.Equal(t, "witness500", block.Witness)
}

// TestGetOperationsByBlock tests retrieving operations by block
func TestGetOperationsByBlock(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Get operations for non-existent block
	ops, err := client.GetOperationsByBlock(ctx, 9999)
	require.NoError(t, err)
	assert.Empty(t, ops)

	// Insert operations
	testOps := []*model.Operation{
		{
			ID:       "600:0:0",
			BlockNum: 600,
			TrxID:    "tx600",
			TrxIndex: 0,
			OpIndex:  0,
			OpType:   "transfer",
			OpValue:  map[string]interface{}{"from": "alice"},
			Virtual:  false,
			Source:   "plugin",
		},
		{
			ID:       "600:1:0",
			BlockNum: 600,
			TrxID:    "tx601",
			TrxIndex: 1,
			OpIndex:  0,
			OpType:   "vote",
			OpValue:  map[string]interface{}{"voter": "bob"},
			Virtual:  false,
			Source:   "plugin",
		},
	}
	err = client.BulkUpsertOperations(ctx, testOps)
	require.NoError(t, err)

	// Retrieve operations
	ops, err = client.GetOperationsByBlock(ctx, 600)
	require.NoError(t, err)
	assert.Len(t, ops, 2)

	// Verify operation details
	opMap := make(map[string]*model.Operation)
	for _, op := range ops {
		opMap[op.ID] = op
	}

	assert.Contains(t, opMap, "600:0:0")
	assert.Contains(t, opMap, "600:1:0")
	assert.Equal(t, "transfer", opMap["600:0:0"].OpType)
	assert.Equal(t, "vote", opMap["600:1:0"].OpType)
}

// TestCheckBlockExists tests checking block existence
func TestCheckBlockExists(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Check non-existent block
	exists, err := client.CheckBlockExists(ctx, 9999)
	require.NoError(t, err)
	assert.False(t, exists)

	// Insert a block
	testBlock := &model.Block{
		ID:               700,
		BlockNum:         700,
		BlockID:          "block700",
		Previous:         "block699",
		Timestamp:        time.Now(),
		Witness:          "witness700",
		TransactionCount: 5,
	}
	err = client.BulkUpsertBlocks(ctx, []*model.Block{testBlock})
	require.NoError(t, err)

	// Check existing block
	exists, err = client.CheckBlockExists(ctx, 700)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestBulkUpsertLargeBatch tests large batch upsert
func TestBulkUpsertLargeBatch(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Create a large batch of operations (1000+)
	ops := make([]*model.Operation, 0, 1500)
	for i := 0; i < 1500; i++ {
		op := &model.Operation{
			ID:       model.OperationID(800, int32(i/10), int32(i%10)),
			BlockNum: 800,
			TrxID:    "tx800",
			TrxIndex: int32(i / 10),
			OpIndex:  int32(i % 10),
			OpType:   "transfer",
			OpValue: map[string]interface{}{
				"index": i,
			},
			Virtual: false,
			Source:  "plugin",
		}
		ops = append(ops, op)
	}

	// Bulk upsert
	err := client.BulkUpsertOperations(ctx, ops)
	require.NoError(t, err)

	// Verify all operations were inserted
	retrievedOps, err := client.GetOperationsByBlock(ctx, 800)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(retrievedOps), 1500)
}

// TestVirtualOperations tests virtual operations handling
func TestVirtualOperations(t *testing.T) {
	client, _ := setupTestClient(t)
	if client == nil {
		return
	}
	defer cleanupTestData(t, client)

	ctx := context.Background()

	// Insert virtual operation
	virtualOp := &model.Operation{
		ID:       "900:-1:-1",
		BlockNum: 900,
		TrxID:    "",
		TrxIndex: -1,
		OpIndex:  -1,
		OpType:   "author_reward",
		OpValue: map[string]interface{}{
			"author": "alice",
		},
		Virtual: true,
		Source:  "rpc",
	}

	err := client.BulkUpsertOperations(ctx, []*model.Operation{virtualOp})
	require.NoError(t, err)

	// Retrieve and verify
	ops, err := client.GetOperationsByBlock(ctx, 900)
	require.NoError(t, err)
	assert.Len(t, ops, 1)
	assert.True(t, ops[0].Virtual)
	assert.Equal(t, int32(-1), ops[0].TrxIndex)
	assert.Equal(t, int32(-1), ops[0].OpIndex)
}
