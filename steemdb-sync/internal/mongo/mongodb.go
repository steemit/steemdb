package mongo

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client wraps MongoDB client and collections
type Client struct {
	client       *mongo.Client
	db           *mongo.Database
	blocks       *mongo.Collection
	transactions *mongo.Collection
	operations   *mongo.Collection
	meta         *mongo.Collection
}

// NewClient creates a new MongoDB client
func NewClient(cfg *config.Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().
		ApplyURI(cfg.Mongo.URI).
		SetMinPoolSize(uint64(cfg.Mongo.MinPoolSize)).
		SetMaxPoolSize(uint64(cfg.Mongo.MaxPoolSize))

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to MongoDB")
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, errors.Wrap(err, "failed to ping MongoDB")
	}

	db := client.Database(cfg.Mongo.Database)

	c := &Client{
		client:       client,
		db:           db,
		blocks:       db.Collection("blocks"),
		transactions: db.Collection("transactions"),
		operations:   db.Collection("operations"),
		meta:         db.Collection("meta"),
	}

	// Create indexes
	if err := c.createIndexes(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to create indexes")
	}

	return c, nil
}

// createIndexes creates all required indexes
func (c *Client) createIndexes(ctx context.Context) error {
	// Blocks indexes
	blocksIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "timestamp", Value: 1}},
		},
	}

	// Transactions indexes
	transactionsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "block_num", Value: 1}},
		},
	}

	// Operations indexes
	operationsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "block_num", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "trx_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "op_type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "virtual", Value: 1}},
		},
		{
			// Serves per-account history queries: equality on the accounts
			// multikey field + descending sort on block_num (monotonic in time).
			Keys: bson.D{{Key: "accounts", Value: 1}, {Key: "block_num", Value: -1}},
		},
	}

	if _, err := c.blocks.Indexes().CreateMany(ctx, blocksIndexes); err != nil {
		return errors.Wrap(err, "failed to create blocks indexes")
	}

	if _, err := c.transactions.Indexes().CreateMany(ctx, transactionsIndexes); err != nil {
		return errors.Wrap(err, "failed to create transactions indexes")
	}

	if _, err := c.operations.Indexes().CreateMany(ctx, operationsIndexes); err != nil {
		return errors.Wrap(err, "failed to create operations indexes")
	}

	// Processor Pattern-B collections are written by multi-field filter upserts
	// (legacy sync.py Pattern B: dedup by business fields). Without an index on
	// the filter fields every upsert is a collection scan — the processor
	// ground at minutes-per-thousand-blocks until these were added.
	patternBIndexes := []struct {
		collection string
		keys       bson.D
	}{
		{"follow", bson.D{{Key: "_block", Value: 1}, {Key: "follower", Value: 1}, {Key: "following", Value: 1}}},
		{"reblog", bson.D{{Key: "_block", Value: 1}, {Key: "permlink", Value: 1}, {Key: "account", Value: 1}}},
		{"witness_vote", bson.D{{Key: "_ts", Value: 1}, {Key: "account", Value: 1}, {Key: "witness", Value: 1}}},
		{"benefactor_reward", bson.D{{Key: "_ts", Value: 1}, {Key: "benefactor", Value: 1}, {Key: "permlink", Value: 1}, {Key: "author", Value: 1}}},
	}
	for _, p := range patternBIndexes {
		if _, err := c.db.Collection(p.collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: p.keys}); err != nil {
			return errors.Wrapf(err, "failed to create %s indexes", p.collection)
		}
	}

	return nil
}

// Close closes the MongoDB connection
func (c *Client) Close(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// BulkUpsertOperations performs bulk upsert of operations
func (c *Client) BulkUpsertOperations(ctx context.Context, ops []*model.Operation) error {
	if len(ops) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(ops))
	for _, op := range ops {
		// Derive involved accounts at the single write choke point so every
		// source (batcher, repair, live_sync) populates the field.
		op.Accounts = model.ExtractAccounts(op.OpValue)
		filter := bson.M{"_id": op.ID}
		update := bson.M{"$set": op}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.operations.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("operations", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert operations")
	}

	return nil
}

// BackfillOperationAccounts scans operations that lack the accounts field,
// derives involved accounts, and writes them back in batches. It is the
// one-time migration path for data ingested before the field existed; newly
// ingested operations are populated automatically by BulkUpsertOperations.
// Already-backfilled documents no longer match the scan filter, so the loop
// terminates without a separate cursor bookmark.
func (c *Client) BackfillOperationAccounts(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}

	var total int64
	for {
		findOptions := options.Find().
			SetLimit(int64(batchSize)).
			SetProjection(bson.M{"op_type": 1, "op_value": 1})

		cursor, err := c.operations.Find(ctx, bson.M{"accounts": bson.M{"$exists": false}}, findOptions)
		if err != nil {
			return total, errors.Wrap(err, "failed to find operations without accounts")
		}

		type opRef struct {
			ID      string                 `bson:"_id"`
			OpValue map[string]interface{} `bson:"op_value"`
		}
		var batch []opRef
		if err := cursor.All(ctx, &batch); err != nil {
			cursor.Close(ctx)
			return total, errors.Wrap(err, "failed to decode operations without accounts")
		}
		cursor.Close(ctx)

		if len(batch) == 0 {
			return total, nil
		}

		models := make([]mongo.WriteModel, 0, len(batch))
		for _, op := range batch {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": op.ID}).
				SetUpdate(bson.M{"$set": bson.M{"accounts": model.ExtractAccounts(op.OpValue)}}))
		}

		opts := options.BulkWrite().SetOrdered(false)
		res, err := c.operations.BulkWrite(ctx, models, opts)
		if err != nil {
			return total, errors.Wrap(err, "failed to backfill accounts")
		}
		total += res.ModifiedCount

		if len(batch) < batchSize {
			return total, nil
		}
	}
}

// BulkUpsertBlocks performs bulk upsert of blocks
func (c *Client) BulkUpsertBlocks(ctx context.Context, blocks []*model.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(blocks))
	for _, block := range blocks {
		filter := bson.M{"_id": block.BlockNum}
		update := bson.M{"$set": block}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.blocks.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("blocks", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert blocks")
	}

	return nil
}

// BulkUpsertTransactions performs bulk upsert of transactions
func (c *Client) BulkUpsertTransactions(ctx context.Context, txs []*model.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	startTime := time.Now()
	models := make([]mongo.WriteModel, 0, len(txs))
	for _, tx := range txs {
		filter := bson.M{"_id": tx.ID}
		update := bson.M{"$set": tx}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true))
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := c.transactions.BulkWrite(ctx, models, opts)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordMongoWrite("transactions", "bulk_upsert", duration, err)

	if err != nil {
		return errors.Wrap(err, "failed to bulk upsert transactions")
	}

	return nil
}

// GetMaxBlock returns the maximum block number from meta collection
func (c *Client) GetMaxBlock(ctx context.Context) (uint32, error) {
	var meta model.Meta
	filter := bson.M{"_id": "sync_state"}
	err := c.meta.FindOne(ctx, filter).Decode(&meta)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Wrap(err, "failed to get max block")
	}
	return meta.MaxBlock, nil
}

// UpdateMaxBlock updates the max block in meta collection
func (c *Client) UpdateMaxBlock(ctx context.Context, blockNum uint32) error {
	filter := bson.M{"_id": "sync_state"}
	update := bson.M{
		"$set": bson.M{
			"max_block":  blockNum,
			"updated_at": time.Now(),
		},
		"$setOnInsert": bson.M{
			"_id":             "sync_state",
			"cold_start_done": false,
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := c.meta.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.Wrap(err, "failed to update max block")
	}

	return nil
}

// SetColdStartDone marks cold start as completed
func (c *Client) SetColdStartDone(ctx context.Context) error {
	filter := bson.M{"_id": "sync_state"}
	update := bson.M{
		"$set": bson.M{
			"cold_start_done": true,
			"updated_at":      time.Now(),
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := c.meta.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.Wrap(err, "failed to set cold start done")
	}

	return nil
}

// GetBlockByNumber retrieves a block by block number
func (c *Client) GetBlockByNumber(ctx context.Context, blockNum uint32) (*model.Block, error) {
	var block model.Block
	filter := bson.M{"_id": blockNum}
	err := c.blocks.FindOne(ctx, filter).Decode(&block)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get block")
	}
	return &block, nil
}

// GetOperationsByBlock retrieves operations for a specific block
func (c *Client) GetOperationsByBlock(ctx context.Context, blockNum uint32) ([]*model.Operation, error) {
	filter := bson.M{"block_num": blockNum}
	cursor, err := c.operations.Find(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find operations")
	}
	defer cursor.Close(ctx)

	var ops []*model.Operation
	if err := cursor.All(ctx, &ops); err != nil {
		return nil, errors.Wrap(err, "failed to decode operations")
	}

	return ops, nil
}

// CheckBlockExists checks if a block exists
func (c *Client) CheckBlockExists(ctx context.Context, blockNum uint32) (bool, error) {
	filter := bson.M{"_id": blockNum}
	count, err := c.blocks.CountDocuments(ctx, filter)
	if err != nil {
		return false, errors.Wrap(err, "failed to check block existence")
	}
	return count > 0, nil
}

// Database returns the underlying *mongo.Database for direct collection access.
// Used by the processor package to write to business collections (account, comment, vote, ...).
func (c *Client) Database() *mongo.Database {
	return c.db
}
