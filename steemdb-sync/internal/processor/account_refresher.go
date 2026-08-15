package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/rpc"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// coldStartThreshold is the number of blocks the processor can be behind
// meta.max_block before the refresher considers itself in "catching up" mode.
const coldStartThreshold uint32 = 1000

// steemDateFormat is the date format used by Steem RPC timestamps.
const steemDateFormat = "2006-01-02T15:04:05"

// AccountRefresher periodically refreshes dirty account data from the Steem RPC.
// It runs as a background goroutine alongside the processor's main loop.
//
// Design: single-threaded + batched RPC calls.
// Every interval (default 30s):
//  1. Check if processor is catching up (skip if so — cold-start semantics)
//  2. Fetch dirty account names from MongoDB (limit batchSize, default 500)
//  3. Call get_accounts in batches of rpcBatchSize (default 100) — single-threaded
//  4. Transform each account's fields and upsert to MongoDB, clearing _dirty
type AccountRefresher struct {
	cfg         *config.Config
	rpcClient   *rpc.Client
	accountCol  *drivermongo.Collection
	statusCol   *drivermongo.Collection
	metaCol     *drivermongo.Collection

	interval     time.Duration
	batchSize    int // dirty accounts per tick (default 500)
	rpcBatchSize int // accounts per get_accounts call (default 100)
}

// NewAccountRefresher creates a new AccountRefresher from the processor context.
func NewAccountRefresher(ctx *Context) (*AccountRefresher, error) {
	interval, err := ctx.Cfg.AccountRefresherInterval()
	if err != nil {
		return nil, fmt.Errorf("invalid account_refresher.interval: %w", err)
	}

	batchSize := ctx.Cfg.Processor.AccountRefresher.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}

	rpcBatchSize := ctx.Cfg.Processor.AccountRefresher.RPCBatchSize
	if rpcBatchSize <= 0 {
		rpcBatchSize = 100
	}

	db := ctx.MongoClient.Database()

	return &AccountRefresher{
		cfg:          ctx.Cfg,
		rpcClient:    ctx.RPCClient,
		accountCol:   db.Collection("account"),
		statusCol:    db.Collection("status"),
		metaCol:      db.Collection("meta"),
		interval:     interval,
		batchSize:    batchSize,
		rpcBatchSize: rpcBatchSize,
	}, nil
}

// Run starts the refresher loop. Blocks until ctx is cancelled.
func (r *AccountRefresher) Run(ctx context.Context) {
	log.Printf("[AccountRefresher] Started (interval=%s, batch_size=%d, rpc_batch_size=%d)",
		r.interval, r.batchSize, r.rpcBatchSize)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[AccountRefresher] Shutting down")
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick performs one refresh cycle.
func (r *AccountRefresher) tick(ctx context.Context) {
	// 1. Skip during cold-start (catching up)
	if r.isCatchingUp(ctx) {
		return
	}

	// 2. Fetch dirty account names
	names := r.fetchDirtyNames(ctx)
	if len(names) == 0 {
		return
	}

	log.Printf("[AccountRefresher] Refreshing %d dirty accounts", len(names))

	// 3. Process in batches of rpcBatchSize
	refreshed := 0
	failed := 0
	for i := 0; i < len(names); i += r.rpcBatchSize {
		end := i + r.rpcBatchSize
		if end > len(names) {
			end = len(names)
		}
		chunk := names[i:end]

		n, errs := r.refreshChunk(ctx, chunk)
		refreshed += n
		failed += errs
	}

	if refreshed > 0 || failed > 0 {
		log.Printf("[AccountRefresher] Refreshed %d accounts (%d failed)", refreshed, failed)
	}
}

// isCatchingUp checks if the processor is still behind the sync cursor.
// During cold-start, get_accounts returns chain-head state (not replay state),
// so refreshing is semantically wrong and wastes RPC calls.
func (r *AccountRefresher) isCatchingUp(ctx context.Context) bool {
	// Get processor cursor height
	processorHeight, err := r.getProcessorHeight(ctx)
	if err != nil {
		log.Printf("[AccountRefresher] Error reading processor height: %v", err)
		return true // fail-safe: skip if we can't read
	}

	// Get max block from meta. If meta is missing or stale (max_block < processorHeight),
	// fall back to the highest block in the blocks collection.
	maxBlock, err := r.getMaxBlock(ctx)
	if err != nil || maxBlock < processorHeight {
		// Fallback: read the latest block from blocks collection
		maxBlock, err = r.getLatestBlockNum(ctx)
		if err != nil {
			log.Printf("[AccountRefresher] Error reading max_block and latest block: %v", err)
			return true
		}
	}

	return maxBlock-processorHeight > coldStartThreshold
}

// fetchDirtyNames retrieves up to batchSize dirty account names from MongoDB.
func (r *AccountRefresher) fetchDirtyNames(ctx context.Context) []string {
	cursor, err := r.accountCol.Find(ctx,
		bson.M{"_dirty": true},
		options.Find().
			SetProjection(bson.M{"_id": 1}).
			SetLimit(int64(r.batchSize)),
	)
	if err != nil {
		log.Printf("[AccountRefresher] Error fetching dirty accounts: %v", err)
		return nil
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID string `bson:"_id"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		log.Printf("[AccountRefresher] Error decoding dirty accounts: %v", err)
		return nil
	}

	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.ID)
	}
	return names
}

// RefreshNames refreshes a specific set of accounts (used by the refresher
// process's optional full rescan). The names are processed in rpcBatchSize
// chunks. Returns (success count, error count).
func (r *AccountRefresher) RefreshNames(ctx context.Context, names []string) (int, int) {
	success := 0
	failed := 0
	for i := 0; i < len(names); i += r.rpcBatchSize {
		end := i + r.rpcBatchSize
		if end > len(names) {
			end = len(names)
		}
		n, errs := r.refreshChunk(ctx, names[i:end])
		success += n
		failed += errs
	}
	return success, failed
}

// refreshChunk calls get_accounts for a batch of names, transforms each, and upserts.
// Returns (success count, error count).
func (r *AccountRefresher) refreshChunk(ctx context.Context, names []string) (int, int) {
	accounts, err := r.rpcClient.GetAccounts(names)
	if err != nil {
		log.Printf("[AccountRefresher] GetAccounts failed (batch=%d): %v", len(names), err)
		return 0, len(names)
	}

	success := 0
	failed := 0
	for _, acct := range accounts {
		doc := processAccount(acct)
		filter := bson.M{"_id": acct.Name}
		update := bson.M{
			"$set":   doc,
			"$unset": bson.M{"_dirty": ""},
		}
		_, err := r.accountCol.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
		if err != nil {
			log.Printf("[AccountRefresher] Failed to upsert account %s: %v", acct.Name, err)
			failed++
		} else {
			success++
		}
	}
	return success, failed
}

// getProcessorHeight reads the processor cursor from the status collection.
func (r *AccountRefresher) getProcessorHeight(ctx context.Context) (uint32, error) {
	var doc struct {
		Value uint32 `bson:"value"`
	}
	err := r.statusCol.FindOne(ctx, bson.M{"_id": "processor_height"}).Decode(&doc)
	if err != nil {
		return 0, err
	}
	return doc.Value, nil
}

// getMaxBlock reads the max_block from the meta collection.
func (r *AccountRefresher) getMaxBlock(ctx context.Context) (uint32, error) {
	var doc struct {
		MaxBlock uint32 `bson:"max_block"`
	}
	err := r.metaCol.FindOne(ctx, bson.M{"_id": "sync_state"}).Decode(&doc)
	if err != nil {
		return 0, err
	}
	return doc.MaxBlock, nil
}

// getLatestBlockNum reads the highest block number from the blocks collection.
// Used as a fallback when meta.sync_state doesn't exist.
func (r *AccountRefresher) getLatestBlockNum(ctx context.Context) (uint32, error) {
	var doc struct {
		ID uint32 `bson:"_id"`
	}
	err := r.statusCol.Database().Collection("blocks").FindOne(ctx,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}),
	).Decode(&doc)
	if err != nil {
		return 0, err
	}
	return doc.ID, nil
}

// --- Account field transformations (mirrors legacy sync.py update_account:383-403) ---

// processAccount transforms a raw ExtendedAccount into a MongoDB document.
func processAccount(acct interface{}) bson.M {
	// Use reflection-free approach: marshal to JSON, then unmarshal to map,
	// then transform fields. This avoids depending on the exact ExtendedAccount
	// struct shape (which is being expanded in steemutil).
	raw, err := json.Marshal(acct)
	if err != nil {
		log.Printf("[AccountRefresher] Failed to marshal account: %v", err)
		return bson.M{}
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Printf("[AccountRefresher] Failed to unmarshal account: %v", err)
		return bson.M{}
	}

	// Transform numeric fields that legacy casts to float
	transformFloatFromRaw(m, "reputation")
	transformFloatFromRaw(m, "to_withdraw")

	// Transform asset strings: "100.000 STEEM" → 100.0 (float)
	transformAssetValue(m, "balance")
	transformAssetValue(m, "sbd_balance")
	transformAssetValue(m, "savings_balance")
	transformAssetValue(m, "savings_sbd_balance")
	transformAssetValue(m, "vesting_balance")
	transformAssetValue(m, "vesting_shares")
	transformAssetValue(m, "delegated_vesting_shares")
	transformAssetValue(m, "received_vesting_shares")
	transformAssetValue(m, "vesting_withdraw_rate")

	// Compute proxy_witness from proxied_vsf_votes[0] / 1000000
	if pvv, ok := m["proxied_vsf_votes"].([]interface{}); ok && len(pvv) >= 1 {
		if val, ok := pvv[0].(float64); ok {
			m["proxy_witness"] = val / 1000000
		}
	}

	// Compute total_balance and total_sbd_balance
	balance := getFloat(m, "balance")
	savingsBalance := getFloat(m, "savings_balance")
	m["total_balance"] = balance + savingsBalance

	sbdBalance := getFloat(m, "sbd_balance")
	savingsSbdBalance := getFloat(m, "savings_sbd_balance")
	m["total_sbd_balance"] = sbdBalance + savingsSbdBalance

	// Parse date fields
	dateFields := []string{
		"created", "last_account_recovery", "last_owner_update",
		"last_post", "last_root_post", "last_vote_time",
		"next_vesting_withdrawal",
		"savings_sbd_last_interest_payment", "savings_sbd_seconds_last_update",
		"sbd_last_interest_payment", "sbd_seconds_last_update",
	}
	for _, f := range dateFields {
		transformDate(m, f)
	}

	// Set scanned timestamp
	m["scanned"] = time.Now().UTC()

	return m
}

// transformAssetValue converts "100.000 STEEM" → 100.0 in place.
func transformAssetValue(m map[string]interface{}, key string) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			m[key] = assetValueFromString(s)
		}
	}
}

// transformFloatFromRaw converts a json.RawMessage or string number to float64.
func transformFloatFromRaw(m map[string]interface{}, key string) {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			m[key] = parseFloatSafe(val)
		case float64:
			// already numeric
		case json.RawMessage:
			m[key] = parseFloatSafe(string(val))
		}
	}
}

// transformDate parses a "YYYY-MM-DDTHH:MM:SS" string into time.Time.
func transformDate(m map[string]interface{}, key string) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			if t, err := time.Parse(steemDateFormat, s); err == nil {
				m[key] = t
			}
		}
	}
}

// getFloat extracts a float64 from a map value.
func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

// assetValueFromString parses "100.000 STEEM" → 100.0.
func assetValueFromString(s string) float64 {
	var parts []string
	current := ""
	for _, c := range s {
		if c == ' ' || c == '\t' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	if len(parts) < 1 {
		return 0
	}
	return parseFloatSafe(parts[0])
}

// parseFloatSafe parses a string to float64, returning 0 on error.
func parseFloatSafe(s string) float64 {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil {
		return 0
	}
	return f
}
