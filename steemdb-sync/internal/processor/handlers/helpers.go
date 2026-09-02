package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoInserter provides upsert helpers for business collections.
// All handlers share one inserter (thread-safe: mongo client is concurrency-safe).
//
// Batch mode: when enabled (BeginBatch), upserts are buffered per collection
// and flushed as one unordered BulkWrite each (FlushAll). Two invariants keep
// this safe (docs/rules/processor-write-ordering.md):
//   - same-document collisions within a window flush the bucket first, so the
//     later op wins;
//   - collections whose writes have read/conditional semantics (comment:
//     diff read-modify-write, reward writeback matched-check) are never
//     buffered — they execute immediately, exactly as in serial mode.
type MongoInserter struct {
	db *mongo.Database

	mu          sync.Mutex
	batchMode   bool
	bufferLimit int
	buckets     map[string]*writeBucket
	// dirtyAccounts coalesces QueueAccountDirty marks to one update per
	// account per window.
	dirtyAccounts map[string]struct{}
}

// writeBucket buffers the pending writes of one collection.
type writeBucket struct {
	models []mongo.WriteModel
	// keys holds canonical filters of pending writes for same-document
	// collision detection.
	keys map[string]struct{}
}

// unbufferedCollections lists collections that always execute immediately in
// batch mode. "comment" is here because its writes interleave with reads and
// matched-count checks (comment diff path, author/curation reward writeback)
// whose outcomes must match serial semantics exactly.
var unbufferedCollections = map[string]bool{
	"comment": true,
}

// NewMongoInserter creates a new inserter backed by the given database.
func NewMongoInserter(db *mongo.Database) *MongoInserter {
	return &MongoInserter{db: db}
}

// BeginBatch enables batch buffering with the given per-collection cap.
func (m *MongoInserter) BeginBatch(bufferLimit int) {
	if bufferLimit <= 0 {
		bufferLimit = 5000
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchMode = true
	m.bufferLimit = bufferLimit
	m.buckets = make(map[string]*writeBucket)
	m.dirtyAccounts = make(map[string]struct{})
}

// EndBatch disables batch mode and discards unflushed buffers. Used on
// shutdown: the cursor has not advanced, so the window replays on restart.
func (m *MongoInserter) EndBatch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchMode = false
	m.buckets = nil
	m.dirtyAccounts = nil
}

// FlushAll writes all buffered models and coalesced dirty-account marks.
// On error the caller must not advance the cursor: the window replays.
func (m *MongoInserter) FlushAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.batchMode {
		return nil
	}
	var firstErr error
	for coll := range m.buckets {
		if err := m.flushBucketLocked(ctx, coll); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := m.flushDirtyLocked(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// flushBucketLocked BulkWrites one collection's pending models. Caller holds mu.
func (m *MongoInserter) flushBucketLocked(ctx context.Context, coll string) error {
	b := m.buckets[coll]
	if b == nil || len(b.models) == 0 {
		return nil
	}
	_, err := m.db.Collection(coll).BulkWrite(ctx, b.models, options.BulkWrite().SetOrdered(false))
	b.models = b.models[:0]
	b.keys = make(map[string]struct{})
	if err != nil {
		return fmt.Errorf("bulk write %s (%d models): %w", coll, len(b.models), err)
	}
	return nil
}

// flushDirtyLocked applies coalesced account dirty marks. Caller holds mu.
func (m *MongoInserter) flushDirtyLocked(ctx context.Context) error {
	if len(m.dirtyAccounts) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(m.dirtyAccounts))
	for name := range m.dirtyAccounts {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": name}).
			SetUpdate(bson.M{"$set": bson.M{"_dirty": true}}).
			SetUpsert(true))
	}
	m.dirtyAccounts = make(map[string]struct{})
	if _, err := m.db.Collection("account").BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
		return fmt.Errorf("bulk write account dirty (%d models): %w", len(models), err)
	}
	return nil
}

// appendModel buffers one upsert, enforcing the write-ordering invariant.
func (m *MongoInserter) appendModel(ctx context.Context, coll string, filter, update bson.M) error {
	if unbufferedCollections[coll] {
		_, err := m.db.Collection(coll).UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.batchMode {
		_, err := m.db.Collection(coll).UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
		return err
	}
	b := m.buckets[coll]
	if b == nil {
		b = &writeBucket{keys: make(map[string]struct{})}
		m.buckets[coll] = b
	}
	key := canonicalFilterKey(filter)
	if _, dup := b.keys[key]; dup {
		// Same-document collision inside the window: flush first so the
		// pending earlier write lands before this one (later op must win).
		if err := m.flushBucketLocked(ctx, coll); err != nil {
			return err
		}
	}
	b.models = append(b.models, mongo.NewUpdateOneModel().
		SetFilter(filter).SetUpdate(update).SetUpsert(true))
	b.keys[key] = struct{}{}
	if len(b.models) >= m.bufferLimit {
		return m.flushBucketLocked(ctx, coll)
	}
	return nil
}

// canonicalFilterKey renders a filter deterministically (sorted keys) so that
// structurally identical filters compare equal.
func canonicalFilterKey(filter bson.M) string {
	b, err := json.Marshal(filter)
	if err != nil {
		return fmt.Sprintf("%v", filter)
	}
	return string(b)
}

// UpsertOne upserts a document into the named collection by _id.
// In batch mode the write is buffered (see MongoInserter doc comment).
func (m *MongoInserter) UpsertOne(ctx context.Context, collection string, id interface{}, doc interface{}) error {
	filter := bson.M{"_id": id}
	update := bson.M{"$set": doc}
	return m.appendModel(ctx, collection, filter, update)
}

// UpdateOne performs a partial update on a document in the named collection.
// Returns whether a document was matched (for callers to detect "not found").
// Always executes immediately, even in batch mode: the matched result feeds
// caller logic, so the write is inherently synchronous.
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
	update := bson.M{"$set": doc}
	return m.appendModel(ctx, collection, filter, update)
}

// QueueAccountDirty marks an account as needing refresh by setting _dirty: true.
// This is the lazy-update pattern from legacy sync.py: handlers don't fetch full
// account state, they just flag it. A separate worker periodically refreshes dirty accounts.
func (m *MongoInserter) QueueAccountDirty(ctx context.Context, accountName string) error {
	if accountName == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.batchMode {
		_, err := m.db.Collection("account").UpdateOne(ctx,
			bson.M{"_id": accountName},
			bson.M{"$set": bson.M{"_dirty": true}},
			options.Update().SetUpsert(true))
		return err
	}
	m.dirtyAccounts[accountName] = struct{}{}
	return nil
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
//
//	"1.000 STEEM"                          → (1.0, "STEEM")
//	{"nai":"@@000000021","amount":"833000","precision":3} → (833.0, "STEEM")
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
