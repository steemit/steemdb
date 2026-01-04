package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/steemit/steemdb/sync/internal/utils"
)

type MongoDB struct {
	client   *mongo.Client
	database *mongo.Database
	config   utils.MongoDBConfig
	logger   utils.Logger
}

// NewMongoDB creates a new MongoDB connection
func NewMongoDB(config utils.MongoDBConfig, logger utils.Logger) (*MongoDB, error) {
	// Create client options
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(uint64(config.PoolSize)).
		SetConnectTimeout(config.Timeout).
		SetServerSelectionTimeout(config.Timeout)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database(config.Database)

	logger.Info("Connected to MongoDB",
		utils.String("uri", config.URI),
		utils.String("database", config.Database),
		utils.Int("pool_size", config.PoolSize),
	)

	return &MongoDB{
		client:   client,
		database: database,
		config:   config,
		logger:   logger,
	}, nil
}

// Close closes the MongoDB connection
func (m *MongoDB) Close(ctx context.Context) error {
	if err := m.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}
	m.logger.Info("Disconnected from MongoDB")
	return nil
}

// GetDatabase returns the database instance
func (m *MongoDB) GetDatabase() *mongo.Database {
	return m.database
}

// GetClient returns the client instance
func (m *MongoDB) GetClient() *mongo.Client {
	return m.client
}

// Collection returns a collection instance
func (m *MongoDB) Collection(name string) *mongo.Collection {
	return m.database.Collection(name)
}

// Ping checks the database connection
func (m *MongoDB) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, readpref.Primary())
}

// HealthCheck performs a health check on the database
func (m *MongoDB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := m.Ping(ctx); err != nil {
		return fmt.Errorf("MongoDB health check failed: %w", err)
	}

	return nil
}

// GetLastProcessedBlock retrieves the last processed block number
func (m *MongoDB) GetLastProcessedBlock(ctx context.Context) (int64, error) {
	collection := m.Collection("status")

	var result struct {
		ID    string `bson:"_id"`
		Value int64  `bson:"value"`
	}

	err := collection.FindOne(ctx, map[string]interface{}{"_id": "height"}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 1, nil // Start from block 1 if no record exists
		}
		return 0, fmt.Errorf("failed to get last processed block: %w", err)
	}

	return result.Value, nil
}

// SaveLastProcessedBlock saves the last processed block number
func (m *MongoDB) SaveLastProcessedBlock(ctx context.Context, blockNum int64) error {
	collection := m.Collection("status")

	filter := map[string]interface{}{"_id": "height"}
	update := map[string]interface{}{
		"$set": map[string]interface{}{
			"value":      blockNum,
			"updated_at": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save last processed block: %w", err)
	}

	return nil
}

// BulkWrite performs bulk write operations
func (m *MongoDB) BulkWrite(ctx context.Context, collection string, operations []mongo.WriteModel) error {
	if len(operations) == 0 {
		return nil
	}

	coll := m.Collection(collection)
	opts := options.BulkWrite().SetOrdered(false)

	_, err := coll.BulkWrite(ctx, operations, opts)
	if err != nil {
		return fmt.Errorf("bulk write failed for collection %s: %w", collection, err)
	}

	return nil
}

// CreateIndexes creates indexes for better performance
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	indexes := map[string][]mongo.IndexModel{
		// blocks collection
		"blocks": {
			{Keys: bson.D{{Key: "number", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "hash", Value: 1}}},                                    // block_id query
			{Keys: bson.D{{Key: "timestamp", Value: -1}}},                              // Latest blocks
			{Keys: bson.D{{Key: "witness", Value: 1}, {Key: "timestamp", Value: -1}}},  // Witness blocks
			{Keys: bson.D{{Key: "date_idx", Value: 1}, {Key: "timestamp", Value: -1}}}, // Date query
		},
		// operations collection
		"operations": {
			{Keys: bson.D{{Key: "block_num", Value: 1}, {Key: "trx_in_block", Value: 1}, {Key: "op_index", Value: 1}}}, // Block operations order
			{Keys: bson.D{{Key: "trx_id", Value: 1}}}, // tx_id query
			{Keys: bson.D{{Key: "op_type", Value: 1}, {Key: "block_time", Value: -1}}},
			{Keys: bson.D{{Key: "primary_account", Value: 1}, {Key: "block_time", Value: -1}}},
			{Keys: bson.D{{Key: "accounts", Value: 1}, {Key: "block_time", Value: -1}}},
			{Keys: bson.D{{Key: "date_idx", Value: 1}, {Key: "hour_idx", Value: 1}}},
		},
		// account_operations collection
		"account_operations": {
			{Keys: bson.D{{Key: "account", Value: 1}, {Key: "block_time", Value: -1}}}, // Most important
			{Keys: bson.D{{Key: "account", Value: 1}, {Key: "op_type", Value: 1}, {Key: "block_time", Value: -1}}},
			{Keys: bson.D{{Key: "account", Value: 1}, {Key: "block_num", Value: -1}}},
		},
		// accounts collection
		"accounts": {
			{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "name_lower", Value: 1}}}, // Search
			{Keys: bson.D{{Key: "reputation", Value: -1}}},
			{Keys: bson.D{{Key: "vesting_shares", Value: -1}}},
			{Keys: bson.D{{Key: "last_post", Value: -1}}},
			{Keys: bson.D{{Key: "needs_update", Value: 1}, {Key: "last_updated", Value: 1}}}, // Accounts needing update
		},
		// comments collection
		"comments": {
			{Keys: bson.D{{Key: "author", Value: 1}, {Key: "created", Value: -1}}},
			{Keys: bson.D{{Key: "category", Value: 1}, {Key: "created", Value: -1}}},
			{Keys: bson.D{{Key: "created", Value: -1}}},
			{Keys: bson.D{{Key: "net_votes", Value: -1}, {Key: "created", Value: -1}}},
			{Keys: bson.D{{Key: "total_payout_value", Value: -1}}},
		},
		// operation_stats collection
		"operation_stats": {
			{Keys: bson.D{{Key: "op_type", Value: 1}, {Key: "date_idx", Value: 1}, {Key: "hour_idx", Value: 1}}},
		},
		// Legacy collections (keep for backward compatibility)
		"account": {
			{Keys: map[string]interface{}{"name": 1}},
			{Keys: map[string]interface{}{"reputation": -1}},
			{Keys: map[string]interface{}{"vesting_shares": -1}},
		},
		"block_30d": {
			{Keys: bson.D{{Key: "_ts", Value: -1}}},
		},
		"comment": {
			{Keys: bson.D{{Key: "author", Value: 1}, {Key: "permlink", Value: 1}}},
			{Keys: bson.D{{Key: "created", Value: -1}}},
			{Keys: bson.D{{Key: "depth", Value: 1}}},
		},
		"vote": {
			{Keys: bson.D{{Key: "voter", Value: 1}}},
			{Keys: bson.D{{Key: "author", Value: 1}, {Key: "permlink", Value: 1}}},
			{Keys: bson.D{{Key: "_ts", Value: -1}}},
		},
		"transfer": {
			{Keys: map[string]interface{}{"from": 1}},
			{Keys: map[string]interface{}{"to": 1}},
			{Keys: map[string]interface{}{"_ts": -1}},
		},
		"author_reward": {
			{Keys: map[string]interface{}{"author": 1}},
			{Keys: map[string]interface{}{"_ts": -1}},
		},
		"curation_reward": {
			{Keys: map[string]interface{}{"curator": 1}},
			{Keys: map[string]interface{}{"_ts": -1}},
		},
		"witness": {
			{Keys: map[string]interface{}{"owner": 1}},
			{Keys: map[string]interface{}{"votes": -1}},
		},
	}

	for collectionName, indexModels := range indexes {
		collection := m.Collection(collectionName)

		if len(indexModels) > 0 {
			_, err := collection.Indexes().CreateMany(ctx, indexModels)
			if err != nil {
				m.logger.Error("Failed to create indexes",
					utils.String("collection", collectionName),
					utils.Error(err),
				)
				continue
			}

			m.logger.Info("Created indexes",
				utils.String("collection", collectionName),
				utils.Int("count", len(indexModels)),
			)
		}
	}

	return nil
}

// FindAccountsNeedingUpdate finds accounts that need to be updated
func (m *MongoDB) FindAccountsNeedingUpdate(ctx context.Context, limit int) ([]*Account, error) {
	collection := m.Collection("accounts")

	filter := map[string]interface{}{
		"needs_update": true,
	}

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSort(map[string]interface{}{"last_updated": 1}) // Oldest first

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find accounts needing update: %w", err)
	}
	defer cursor.Close(ctx)

	var accounts []*Account
	if err := cursor.All(ctx, &accounts); err != nil {
		return nil, fmt.Errorf("failed to decode accounts: %w", err)
	}

	return accounts, nil
}

// UpsertAccount upserts an account
func (m *MongoDB) UpsertAccount(ctx context.Context, account *Account) error {
	collection := m.Collection("accounts")

	// Set name_lower for search
	account.NameLower = strings.ToLower(account.Name)

	filter := map[string]interface{}{"_id": account.ID}
	update := map[string]interface{}{"$set": account}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert account: %w", err)
	}

	return nil
}

// MarkAccountNeedsUpdate marks an account as needing update
// Reference: legacy/docker/sync/sync.py queue_update_account()
// Legacy only sets _dirty: True, but we need to set name field to avoid unique index violation
// Note: Steem account names are always lowercase, so name_lower == name, but we keep name_lower for search index
func (m *MongoDB) MarkAccountNeedsUpdate(ctx context.Context, accountName string) error {
	collection := m.Collection("accounts")

	// Steem account names are always lowercase per blockchain rules
	// But we still normalize to ensure consistency and set name_lower for search index
	nameLower := strings.ToLower(accountName)

	filter := map[string]interface{}{"_id": accountName}
	update := map[string]interface{}{
		"$set": map[string]interface{}{
			"needs_update": true,
			"name":         accountName, // Set name field to avoid unique index violation (name_1 unique index)
			"name_lower":   nameLower,   // For search index consistency (name_lower index)
		},
		"$setOnInsert": map[string]interface{}{
			"_id": accountName, // Ensure _id is set on insert
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to mark account needs update: %w", err)
	}

	return nil
}

// ============================================================================
// Layer 1: Raw Operation Sync - Collection Sharding Support
// ============================================================================

const (
	// CollectionRangeSize is the block range size for each operations collection
	// Each collection stores 10 million blocks (e.g., operations_0_10000000)
	CollectionRangeSize = int64(10_000_000)
)

// GetBlocksCollectionName returns the collection name for a given block number
// Collection naming: blocks_{start_block}_{end_block}
// Example: blocks_0_10000000, blocks_10000000_20000000
func GetBlocksCollectionName(blockNum int64) string {
	startBlock := (blockNum / CollectionRangeSize) * CollectionRangeSize
	endBlock := startBlock + CollectionRangeSize
	return fmt.Sprintf("blocks_%d_%d", startBlock, endBlock)
}

// GetTransactionsCollectionName returns the collection name for a given block number
// Collection naming: transactions_{start_block}_{end_block}
// Example: transactions_0_10000000, transactions_10000000_20000000
func GetTransactionsCollectionName(blockNum int64) string {
	startBlock := (blockNum / CollectionRangeSize) * CollectionRangeSize
	endBlock := startBlock + CollectionRangeSize
	return fmt.Sprintf("transactions_%d_%d", startBlock, endBlock)
}

// GetOperationsCollectionName returns the collection name for a given block number
// Collection naming: operations_{start_block}_{end_block}
// Example: operations_0_10000000, operations_10000000_20000000
func GetOperationsCollectionName(blockNum int64) string {
	startBlock := (blockNum / CollectionRangeSize) * CollectionRangeSize
	endBlock := startBlock + CollectionRangeSize
	return fmt.Sprintf("operations_%d_%d", startBlock, endBlock)
}

// GetOperationsCollection returns the MongoDB collection for a given block number
func (m *MongoDB) GetOperationsCollection(blockNum int64) *mongo.Collection {
	collectionName := GetOperationsCollectionName(blockNum)
	return m.database.Collection(collectionName)
}

// GetCollectionsInRange returns all collection names that cover the given block range
// Returns collections for operations (can be extended for blocks/transactions)
func GetCollectionsInRange(startBlock, endBlock int64) []string {
	collections := make(map[string]bool)
	
	// Calculate start and end collection ranges
	startCollectionStart := (startBlock / CollectionRangeSize) * CollectionRangeSize
	endCollectionStart := (endBlock / CollectionRangeSize) * CollectionRangeSize
	
	// Add all collections in the range
	for collectionStart := startCollectionStart; collectionStart <= endCollectionStart; collectionStart += CollectionRangeSize {
		collectionEnd := collectionStart + CollectionRangeSize
		collectionName := fmt.Sprintf("operations_%d_%d", collectionStart, collectionEnd)
		collections[collectionName] = true
	}
	
	result := make([]string, 0, len(collections))
	for name := range collections {
		result = append(result, name)
	}
	return result
}

// EnsureCollectionIndexes ensures that indexes exist for a given collection
// This is called before inserting data to ensure indexes are created
func (m *MongoDB) EnsureCollectionIndexes(ctx context.Context, collectionName string) error {
	collection := m.database.Collection(collectionName)
	
	indexes := []mongo.IndexModel{
		// Unique index: block_num + trx_id + op_in_trx + is_virtual + virtual_op_num
		{
			Keys: bson.D{
				{Key: "block_num", Value: 1},
				{Key: "trx_id", Value: 1},
				{Key: "op_in_trx", Value: 1},
				{Key: "is_virtual", Value: 1},
				{Key: "virtual_op_num", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		// Query indexes
		{Keys: bson.D{{Key: "block_num", Value: 1}, {Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}},
		{Keys: bson.D{{Key: "trx_id", Value: 1}}},
		{Keys: bson.D{{Key: "op_type", Value: 1}}},
	}
	
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		// Check if error is due to existing indexes (ignore if so)
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("failed to create indexes for collection %s: %w", collectionName, err)
		}
	}
	
	return nil
}

// InsertOperations inserts multiple operations into MongoDB with sharding support
// Automatically routes to the correct collection based on block number
// Ensures indexes exist before inserting
func (m *MongoDB) InsertOperations(ctx context.Context, ops []*RawOperation) error {
	if len(ops) == 0 {
		return nil
	}
	
	// Group operations by collection
	opsByCollection := make(map[string][]*RawOperation)
	for _, op := range ops {
		collectionName := GetOperationsCollectionName(op.BlockNum)
		opsByCollection[collectionName] = append(opsByCollection[collectionName], op)
	}
	
	// Insert operations for each collection
	for collectionName, collectionOps := range opsByCollection {
		// Ensure indexes exist before inserting
		if err := m.EnsureCollectionIndexes(ctx, collectionName); err != nil {
			return fmt.Errorf("failed to ensure indexes for collection %s: %w", collectionName, err)
		}
		
		collection := m.database.Collection(collectionName)
		now := time.Now()
		
		// Prepare bulk write operations (upsert to prevent duplicates)
		models := make([]mongo.WriteModel, 0, len(collectionOps))
		for _, op := range collectionOps {
			op.CreatedAt = now
			
			filter := bson.M{
				"block_num":      op.BlockNum,
				"trx_id":         op.TrxID,
				"op_in_trx":      op.OpInTrx,
				"is_virtual":     op.IsVirtual,
				"virtual_op_num": op.VirtualOpNum,
			}
			
			update := bson.M{"$set": op}
			upsertModel := mongo.NewUpdateOneModel().
				SetFilter(filter).
				SetUpdate(update).
				SetUpsert(true)
			models = append(models, upsertModel)
		}
		
		opts := options.BulkWrite().SetOrdered(false)
		if _, err := collection.BulkWrite(ctx, models, opts); err != nil {
			return fmt.Errorf("failed to bulk write operations to collection %s: %w", collectionName, err)
		}
	}
	
	return nil
}

// InsertBlocks inserts multiple blocks into MongoDB with sharding support
func (m *MongoDB) InsertBlocks(ctx context.Context, blocks []*RawBlock) error {
	if len(blocks) == 0 {
		return nil
	}
	
	// Group blocks by collection
	blocksByCollection := make(map[string][]*RawBlock)
	for _, block := range blocks {
		collectionName := GetBlocksCollectionName(block.Number)
		blocksByCollection[collectionName] = append(blocksByCollection[collectionName], block)
	}
	
	// Insert blocks for each collection
	for collectionName, collectionBlocks := range blocksByCollection {
		collection := m.database.Collection(collectionName)
		now := time.Now()
		
		// Prepare bulk write operations (upsert to prevent duplicates)
		models := make([]mongo.WriteModel, 0, len(collectionBlocks))
		for _, block := range collectionBlocks {
			block.CreatedAt = now
			
			filter := bson.M{"_id": block.Number}
			update := bson.M{"$set": block}
			upsertModel := mongo.NewUpdateOneModel().
				SetFilter(filter).
				SetUpdate(update).
				SetUpsert(true)
			models = append(models, upsertModel)
		}
		
		opts := options.BulkWrite().SetOrdered(false)
		if _, err := collection.BulkWrite(ctx, models, opts); err != nil {
			return fmt.Errorf("failed to bulk write blocks to collection %s: %w", collectionName, err)
		}
		
		// Ensure indexes exist
		if err := m.EnsureBlocksCollectionIndexes(ctx, collectionName); err != nil {
			m.logger.Warn("Failed to ensure indexes for blocks collection",
				utils.String("collection", collectionName),
				utils.Error(err),
			)
		}
	}
	
	return nil
}

// InsertTransactions inserts multiple transactions into MongoDB with sharding support
func (m *MongoDB) InsertTransactions(ctx context.Context, transactions []*RawTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	
	// Group transactions by collection
	txsByCollection := make(map[string][]*RawTransaction)
	for _, tx := range transactions {
		collectionName := GetTransactionsCollectionName(tx.BlockNum)
		txsByCollection[collectionName] = append(txsByCollection[collectionName], tx)
	}
	
	// Insert transactions for each collection
	for collectionName, collectionTxs := range txsByCollection {
		collection := m.database.Collection(collectionName)
		now := time.Now()
		
		// Prepare bulk write operations (upsert to prevent duplicates)
		models := make([]mongo.WriteModel, 0, len(collectionTxs))
		for _, tx := range collectionTxs {
			tx.CreatedAt = now
			
			filter := bson.M{
				"block_num": tx.BlockNum,
				"trx_id":    tx.TrxID,
			}
			update := bson.M{"$set": tx}
			upsertModel := mongo.NewUpdateOneModel().
				SetFilter(filter).
				SetUpdate(update).
				SetUpsert(true)
			models = append(models, upsertModel)
		}
		
		opts := options.BulkWrite().SetOrdered(false)
		if _, err := collection.BulkWrite(ctx, models, opts); err != nil {
			return fmt.Errorf("failed to bulk write transactions to collection %s: %w", collectionName, err)
		}
		
		// Ensure indexes exist
		if err := m.EnsureTransactionsCollectionIndexes(ctx, collectionName); err != nil {
			m.logger.Warn("Failed to ensure indexes for transactions collection",
				utils.String("collection", collectionName),
				utils.Error(err),
			)
		}
	}
	
	return nil
}

// EnsureBlocksCollectionIndexes ensures that indexes exist for a blocks collection
func (m *MongoDB) EnsureBlocksCollectionIndexes(ctx context.Context, collectionName string) error {
	collection := m.database.Collection(collectionName)
	
	indexes := []mongo.IndexModel{
		// Unique index on block number (already unique via _id)
		{Keys: bson.D{{Key: "block_num", Value: 1}}},
		// Query indexes
		{Keys: bson.D{{Key: "timestamp", Value: -1}}},
		{Keys: bson.D{{Key: "witness", Value: 1}}},
		{Keys: bson.D{{Key: "block_id", Value: 1}}},
	}
	
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("failed to create indexes for collection %s: %w", collectionName, err)
		}
	}
	
	return nil
}

// EnsureTransactionsCollectionIndexes ensures that indexes exist for a transactions collection
func (m *MongoDB) EnsureTransactionsCollectionIndexes(ctx context.Context, collectionName string) error {
	collection := m.database.Collection(collectionName)
	
	indexes := []mongo.IndexModel{
		// Unique index: block_num + trx_id
		{
			Keys: bson.D{
				{Key: "block_num", Value: 1},
				{Key: "trx_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		// Query indexes
		{Keys: bson.D{{Key: "block_num", Value: 1}}},
		{Keys: bson.D{{Key: "trx_id", Value: 1}}},
	}
	
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("failed to create indexes for collection %s: %w", collectionName, err)
		}
	}
	
	return nil
}

// querySingleCollection queries operations from a single collection
func (m *MongoDB) querySingleCollection(ctx context.Context, collectionName string, startBlock, endBlock int64, filter bson.M) ([]*RawOperation, error) {
	collection := m.database.Collection(collectionName)
	
	// Build query filter
	queryFilter := bson.M{
		"block_num": bson.M{
			"$gte": startBlock,
			"$lte": endBlock,
		},
	}
	
	// Merge with additional filters
	for k, v := range filter {
		queryFilter[k] = v
	}
	
	// Query with sort by timestamp
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: 1}})
	
	cursor, err := collection.Find(ctx, queryFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query collection %s: %w", collectionName, err)
	}
	defer cursor.Close(ctx)
	
	var operations []*RawOperation
	if err := cursor.All(ctx, &operations); err != nil {
		return nil, fmt.Errorf("failed to decode operations from collection %s: %w", collectionName, err)
	}
	
	return operations, nil
}

// QueryOperations queries operations from a single collection or multiple collections
// Automatically handles single-collection or cross-collection queries
func (m *MongoDB) QueryOperations(ctx context.Context, startBlock, endBlock int64, filter bson.M) ([]*RawOperation, error) {
	// Determine which collections to query
	collections := GetCollectionsInRange(startBlock, endBlock)
	
	// If only one collection, query directly
	if len(collections) == 1 {
		return m.querySingleCollection(ctx, collections[0], startBlock, endBlock, filter)
	}
	
	// Multiple collections: query in parallel and merge results
	return m.QueryOperationsAcrossCollections(ctx, startBlock, endBlock, filter)
}

// QueryOperationsAcrossCollections queries operations across multiple collections
// Used for backfill or large range queries
func (m *MongoDB) QueryOperationsAcrossCollections(ctx context.Context, startBlock, endBlock int64, filter bson.M) ([]*RawOperation, error) {
	collections := GetCollectionsInRange(startBlock, endBlock)
	
	// Query all collections in parallel (using goroutines)
	type result struct {
		ops []*RawOperation
		err error
	}
	
	results := make(chan result, len(collections))
	
	for _, collectionName := range collections {
		go func(name string) {
			ops, err := m.querySingleCollection(ctx, name, startBlock, endBlock, filter)
			results <- result{ops: ops, err: err}
		}(collectionName)
	}
	
	// Collect results
	var allOperations []*RawOperation
	for i := 0; i < len(collections); i++ {
		res := <-results
		if res.err != nil {
			return nil, res.err
		}
		allOperations = append(allOperations, res.ops...)
	}
	
	// Sort by timestamp (blockchain time order)
	sort.Slice(allOperations, func(i, j int) bool {
		return allOperations[i].Timestamp.Before(allOperations[j].Timestamp)
	})
	
	return allOperations, nil
}

// GetSyncState retrieves the current sync state
func (m *MongoDB) GetSyncState(ctx context.Context) (*SyncState, error) {
	collection := m.Collection("sync_state")
	
	var state SyncState
	err := collection.FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		// Return default state if not found
		return &SyncState{
			ID:                    "current",
			LastBlock:             0,
			LastIrreversibleBlock: 0,
			UpdatedAt:             time.Now(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get sync state: %w", err)
	}
	return &state, nil
}

// UpdateSyncState updates the sync state using $max to ensure last_block only increases
func (m *MongoDB) UpdateSyncState(ctx context.Context, lastBlock, lastIrreversibleBlock int64) error {
	collection := m.Collection("sync_state")
	
	filter := bson.M{}
	update := bson.M{
		"$set": bson.M{
			"last_irreversible_block": lastIrreversibleBlock,
			"updated_at":               time.Now(),
		},
		"$max": bson.M{
			"last_block": lastBlock,
		},
		"$setOnInsert": bson.M{
			"_id": "current",
		},
	}
	
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to update sync state: %w", err)
	}
	
	return nil
}

// ============================================================================
// Layer 2: Business Processing State Management
// ============================================================================

// GetBusinessProcessingState retrieves the business processing state for a given business type
func (m *MongoDB) GetBusinessProcessingState(ctx context.Context, businessType string) (*BusinessProcessingState, error) {
	collection := m.Collection("business_processing_state")
	
	var state BusinessProcessingState
	err := collection.FindOne(ctx, bson.M{"_id": businessType}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		// Return default state if not found
		return &BusinessProcessingState{
			ID:        businessType,
			LastBlock: 0,
			UpdatedAt: time.Now(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get business processing state for %s: %w", businessType, err)
	}
	return &state, nil
}

// UpdateBusinessProcessingState updates the business processing state
func (m *MongoDB) UpdateBusinessProcessingState(ctx context.Context, businessType string, lastBlock int64) error {
	collection := m.Collection("business_processing_state")
	
	filter := bson.M{"_id": businessType}
	update := bson.M{
		"$set": bson.M{
			"last_block": lastBlock,
			"updated_at": time.Now(),
		},
		"$setOnInsert": bson.M{
			"_id": businessType,
		},
	}
	
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to update business processing state for %s: %w", businessType, err)
	}
	
	return nil
}
