package database

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// MongoDB represents MongoDB connection and operations
type MongoDB struct {
	client   *mongo.Client
	database *mongo.Database
	config   utils.MongoDBConfig
	logger   utils.Logger
}

// NewMongoDB creates a new MongoDB connection
func NewMongoDB(config utils.MongoDBConfig, logger utils.Logger) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	// Log the URI being used for debugging
	logger.Info("Connecting to MongoDB",
		utils.String("uri", config.URI),
		utils.String("database", config.Database))

	// Set client options
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(uint64(config.PoolSize)).
		SetConnectTimeout(config.Timeout).
		SetServerSelectionTimeout(config.Timeout)

	// Connect to MongoDB
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
		utils.String("database", config.Database),
		utils.String("uri", config.URI))

	return &MongoDB{
		client:   client,
		database: database,
		config:   config,
		logger:   logger,
	}, nil
}

// Close closes the MongoDB connection
func (m *MongoDB) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

// Ping checks if the connection is alive
func (m *MongoDB) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, readpref.Primary())
}

// Collection returns a collection handle
func (m *MongoDB) Collection(name string) *mongo.Collection {
	return m.database.Collection(name)
}

// Database returns the database handle
func (m *MongoDB) Database() *mongo.Database {
	return m.database
}

// Client returns the client handle
func (m *MongoDB) Client() *mongo.Client {
	return m.client
}

// BulkWrite performs bulk write operations
func (m *MongoDB) BulkWrite(ctx context.Context, collectionName string, operations []mongo.WriteModel) error {
	if len(operations) == 0 {
		return nil
	}

	collection := m.Collection(collectionName)
	opts := options.BulkWrite().SetOrdered(false)

	result, err := collection.BulkWrite(ctx, operations, opts)
	if err != nil {
		return fmt.Errorf("bulk write failed: %w", err)
	}

	m.logger.Debug("Bulk write completed",
		utils.String("collection", collectionName),
		utils.Int64("inserted", result.InsertedCount),
		utils.Int64("modified", result.ModifiedCount),
		utils.Int64("deleted", result.DeletedCount),
		utils.Int64("upserted", result.UpsertedCount))

	return nil
}

// Transaction executes operations within a transaction
func (m *MongoDB) Transaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	session, err := m.client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	return mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		_, err := session.WithTransaction(sc, func(sc mongo.SessionContext) (interface{}, error) {
			return nil, fn(sc)
		})
		return err
	})
}

// CreateIndexes creates indexes for collections
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	indexes := map[string][]mongo.IndexModel{
		"account": {
			{Keys: map[string]interface{}{"name": 1}},
			{Keys: map[string]interface{}{"reputation": -1}},
			{Keys: map[string]interface{}{"vesting_shares": -1}},
			{Keys: map[string]interface{}{"last_update": -1}},
		},
		"blocks": {
			{Keys: map[string]interface{}{"timestamp": -1}},
			{Keys: map[string]interface{}{"witness": 1}},
		},
		"comment": {
			{Keys: map[string]interface{}{"author": 1, "permlink": 1}},
			{Keys: map[string]interface{}{"category": 1}},
			{Keys: map[string]interface{}{"created": -1}},
			{Keys: map[string]interface{}{"net_votes": -1}},
			{Keys: map[string]interface{}{"pending_payout_value": -1}},
		},
		"vote": {
			{Keys: map[string]interface{}{"voter": 1}},
			{Keys: map[string]interface{}{"author": 1, "permlink": 1}},
			{Keys: map[string]interface{}{"timestamp": -1}},
		},
		"witness": {
			{Keys: map[string]interface{}{"owner": 1}},
			{Keys: map[string]interface{}{"votes": -1}},
			{Keys: map[string]interface{}{"total_missed": 1}},
		},
		"transfer": {
			{Keys: map[string]interface{}{"from": 1}},
			{Keys: map[string]interface{}{"to": 1}},
			{Keys: map[string]interface{}{"timestamp": -1}},
		},
	}

	for collectionName, indexModels := range indexes {
		collection := m.Collection(collectionName)

		if len(indexModels) > 0 {
			_, err := collection.Indexes().CreateMany(ctx, indexModels)
			if err != nil {
				m.logger.Warn("Failed to create indexes",
					utils.String("collection", collectionName),
					utils.Error(err))
			} else {
				m.logger.Info("Created indexes",
					utils.String("collection", collectionName),
					utils.Int("count", len(indexModels)))
			}
		}
	}

	return nil
}

// GetStats returns database statistics
func (m *MongoDB) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := m.database.RunCommand(ctx, map[string]interface{}{"dbStats": 1}).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to get database stats: %w", err)
	}
	return result, nil
}
