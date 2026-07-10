package handlers

import (
	"context"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoInserter provides upsert helpers for business collections.
// All handlers share one inserter (thread-safe: mongo client is concurrency-safe).
type MongoInserter struct {
	db *mongo.Database
}

// NewMongoInserter creates a new inserter backed by the given database.
func NewMongoInserter(db *mongo.Database) *MongoInserter {
	return &MongoInserter{db: db}
}

// UpsertOne upserts a document into the named collection by _id.
func (m *MongoInserter) UpsertOne(ctx context.Context, collection string, id interface{}, doc interface{}) error {
	col := m.db.Collection(collection)
	filter := bson.M{"_id": id}
	update := bson.M{"$set": doc}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
}

// UpdateOne performs a partial update on a document in the named collection.
// Returns whether a document was matched (for callers to detect "not found").
func (m *MongoInserter) UpdateOne(ctx context.Context, collection string, filter bson.M, update bson.M) (bool, error) {
	col := m.db.Collection(collection)
	result, err := col.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

// QueueAccountDirty marks an account as needing refresh by setting _dirty: true.
// This is the lazy-update pattern from legacy sync.py: handlers don't fetch full
// account state, they just flag it. A separate worker periodically refreshes dirty accounts.
func (m *MongoInserter) QueueAccountDirty(ctx context.Context, accountName string) error {
	if accountName == "" {
		return nil
	}
	col := m.db.Collection("account")
	filter := bson.M{"_id": accountName}
	update := bson.M{
		"$set": bson.M{"_dirty": true},
	}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
}

// --- Amount / type conversion helpers (mirrors legacy sync.py) ---

// SplitAmount parses a Steem amount string like "1.000 STEEM" into (value, unit).
func SplitAmount(s string) (float64, string) {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0, ""
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, parts[1]
	}
	return val, parts[1]
}

// AmountValue extracts just the numeric part of an amount string.
func AmountValue(s string) float64 {
	v, _ := SplitAmount(s)
	return v
}

// AmountUnit extracts just the unit part of an amount string.
func AmountUnit(s string) string {
	_, unit := SplitAmount(s)
	return unit
}

// GetString reads a string field from a map, returning "" if missing or wrong type.
func GetString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
