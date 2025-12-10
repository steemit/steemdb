package database

import (
	"context"
	"fmt"
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
