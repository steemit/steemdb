package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/steemdb/sync/internal/utils"
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
func (m *MongoDB) MarkAccountNeedsUpdate(ctx context.Context, accountName string) error {
	collection := m.Collection("accounts")

	filter := map[string]interface{}{"_id": accountName}
	update := map[string]interface{}{
		"$set": map[string]interface{}{
			"needs_update": true,
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to mark account needs update: %w", err)
	}

	return nil
}
