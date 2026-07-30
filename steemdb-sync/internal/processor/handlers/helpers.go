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

// UpsertOneByFilter upserts a document using a multi-field filter (not just _id).
// Used by handlers that follow legacy sync.py Pattern B (benefactor_reward, reblog,
// follow, witness_vote). The filter fields should also be present in the doc so that
// upsert-insert creates a complete document.
func (m *MongoInserter) UpsertOneByFilter(ctx context.Context, collection string, filter bson.M, doc bson.M) error {
	col := m.db.Collection(collection)
	update := bson.M{"$set": doc}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
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

// --- Asset / amount conversion helpers ---
//
// Steem amounts can appear in two formats depending on the data source:
//
//  1. String format (from live_sync / RPC / condenser_api):
//     "1.000 STEEM", "2.500 SBD", "1234.567890 VESTS"
//
//  2. NAI object format (from cold_ingest / ingest plugin / C++ fc::variant):
//     {"nai":"@@000000021", "amount":"833000", "precision":3}
//
// The processor must handle both because the operations collection contains
// a mix of plugin-sourced and rpc-sourced data.

// naiToSymbol maps Steem NAI constants to their human-readable symbols.
// These are fixed on the Steem chain and never change.
// Reference: https://developers.steem.io/apiref/transactions.html#_amount_format
var naiToSymbol = map[string]string{
	"@@000000021": "STEEM", // precision 3
	"@@000000013": "SBD",   // precision 3
	"@@000000037": "VESTS", // precision 6
}

// ParseAsset parses an amount field that may be a string or NAI object.
// Returns (value, symbol). On any parse failure returns (0, "").
//
// Examples:
//   "1.000 STEEM"                          → (1.0, "STEEM")
//   {"nai":"@@000000021","amount":"833000","precision":3} → (833.0, "STEEM")
func ParseAsset(v interface{}) (float64, string) {
	if v == nil {
		return 0, ""
	}

	// Try string format: "1.000 STEEM"
	if s, ok := v.(string); ok {
		return parseStringAsset(s)
	}

	// Try NAI object format: map[string]interface{}
	if m, ok := v.(map[string]interface{}); ok {
		return parseNAIAsset(m)
	}

	return 0, ""
}

// parseStringAsset handles "1.000 STEEM" format.
func parseStringAsset(s string) (float64, string) {
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

// parseNAIAsset handles {"nai":"@@000000021","amount":"833000","precision":3} format.
func parseNAIAsset(m map[string]interface{}) (float64, string) {
	// Get raw integer amount (e.g. "833000")
	amountStr := ""
	if v, ok := m["amount"]; ok {
		if s, ok := v.(string); ok {
			amountStr = s
		}
	}

	// Get precision to divide the integer (e.g. precision 3 → divide by 1000)
	precision := 0
	if v, ok := m["precision"]; ok {
		switch p := v.(type) {
		case float64:
			precision = int(p)
		case int:
			precision = p
		case int64:
			precision = int(p)
		}
	}

	// Get symbol from NAI lookup
	symbol := ""
	if v, ok := m["nai"]; ok {
		if nai, ok := v.(string); ok {
			symbol = naiToSymbol[nai]
		}
	}

	// Parse the integer amount
	rawVal, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, symbol
	}

	// Apply precision divisor
	divisor := 1.0
	for i := 0; i < precision; i++ {
		divisor *= 10
	}
	return rawVal / divisor, symbol
}

// AssetValue extracts just the numeric value from an amount field (any format).
func AssetValue(v interface{}) float64 {
	val, _ := ParseAsset(v)
	return val
}

// AssetSymbol extracts just the symbol/unit from an amount field (any format).
func AssetSymbol(v interface{}) string {
	_, sym := ParseAsset(v)
	return sym
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

// GetField reads any field from a map, returning nil if missing.
func GetField(m map[string]interface{}, key string) interface{} {
	if v, ok := m[key]; ok {
		return v
	}
	return nil
}
