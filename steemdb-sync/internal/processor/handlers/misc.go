package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// FeedPublishHandler processes "feed_publish" operations
// → writes to the "feed_publish" collection.
// Mirrors legacy sync.py:save_feed_publish (line 197).
//
// _id format: "{blockid}|{publisher}" — note the pipe separator.
type FeedPublishHandler struct {
	inserter *MongoInserter
}

// NewFeedPublishHandler creates a new FeedPublishHandler.
func NewFeedPublishHandler(inserter *MongoInserter) *FeedPublishHandler {
	return &FeedPublishHandler{inserter: inserter}
}

// Handle processes a feed_publish operation.
func (h *FeedPublishHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	publisher := GetString(v, "publisher")

	id := fmt.Sprintf("%d|%s", op.BlockNum, publisher)

	// Legacy mutates exchange_rate.base and exchange_rate.quote in place:
	// takes the numeric value (split()[0] → float), overwriting the string.
	doc := bson.M{
		"_id":       id,
		"_block":    op.BlockNum,
		"_ts":       blockTS,
		"publisher": publisher,
	}

	// Process nested exchange_rate if present
	if er, ok := v["exchange_rate"].(map[string]interface{}); ok {
		erCopy := make(map[string]interface{})
		for k, val := range er {
			erCopy[k] = val
		}
		// Parse base and quote as asset values
		if base, exists := erCopy["base"]; exists {
			erCopy["base"] = AssetValue(base)
		}
		if quote, exists := erCopy["quote"]; exists {
			erCopy["quote"] = AssetValue(quote)
		}
		doc["exchange_rate"] = erCopy
	}

	// Preserve other fields
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "feed_publish", id, doc); err != nil {
		return fmt.Errorf("failed to upsert feed_publish %s: %w", id, err)
	}

	// Legacy does NOT queue_update_account for feed_publish
	return nil
}

// WitnessVoteHandler processes "account_witness_vote" operations
// → writes to the "witness_vote" collection.
// Mirrors legacy sync.py:save_witness_vote (line 288).
//
// Uses Pattern B: filter includes _ts (unusual — same account→witness vote
// in different blocks creates separate documents).
type WitnessVoteHandler struct {
	inserter *MongoInserter
}

// NewWitnessVoteHandler creates a new WitnessVoteHandler.
func NewWitnessVoteHandler(inserter *MongoInserter) *WitnessVoteHandler {
	return &WitnessVoteHandler{inserter: inserter}
}

// Handle processes an account_witness_vote operation.
func (h *WitnessVoteHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	account := GetString(v, "account")
	witness := GetString(v, "witness")

	// Legacy includes _ts in the filter — reuse the same timestamp for filter and doc
	filter := bson.M{
		"_ts":     blockTS,
		"account": account,
		"witness": witness,
	}

	doc := bson.M{
		"_ts":     blockTS,
		"account": account,
		"witness": witness,
	}
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOneByFilter(ctx, "witness_vote", filter, doc); err != nil {
		return fmt.Errorf("failed to upsert witness_vote: %w", err)
	}

	if err := h.inserter.QueueAccountDirty(ctx, account); err != nil {
		return fmt.Errorf("failed to queue account dirty (account=%s): %w", account, err)
	}
	if account != witness {
		if err := h.inserter.QueueAccountDirty(ctx, witness); err != nil {
			return fmt.Errorf("failed to queue account dirty (witness=%s): %w", witness, err)
		}
	}

	return nil
}

// PowHandler processes "pow" and "pow2" operations
// → writes to the "pow" collection.
// Mirrors legacy sync.py:save_pow (line 265).
// Register this handler for BOTH "pow" and "pow2" op_types.
//
// _id format: "{blockid}-{worker_account}"
// worker_account extraction differs:
//   - pow2: work is a list → work[1].input.worker_account
//   - pow:  work is a map  → top-level worker_account field
type PowHandler struct {
	inserter *MongoInserter
}

// NewPowHandler creates a new PowHandler.
func NewPowHandler(inserter *MongoInserter) *PowHandler {
	return &PowHandler{inserter: inserter}
}

// Handle processes a pow or pow2 operation.
func (h *PowHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue

	workerAccount := extractWorkerAccount(v)
	id := fmt.Sprintf("%d-%s", op.BlockNum, workerAccount)

	doc := bson.M{
		"_id":   id,
		"_ts":   blockTS,
		"block": op.BlockNum, // NOTE: field name is "block", not "_block"
	}
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "pow", id, doc); err != nil {
		return fmt.Errorf("failed to upsert pow %s: %w", id, err)
	}

	// Legacy does NOT queue_update_account for pow
	return nil
}

// extractWorkerAccount gets the worker account from a pow/pow2 operation.
// pow2: work is a list [props, {input: {worker_account: ...}}]
// pow:  work is a map, worker_account is a top-level field
func extractWorkerAccount(v map[string]interface{}) string {
	// Check if work is a list (pow2 format)
	if work, ok := v["work"].([]interface{}); ok && len(work) >= 2 {
		if inner, ok := work[1].(map[string]interface{}); ok {
			if input, ok := inner["input"].(map[string]interface{}); ok {
				if account, ok := input["worker_account"].(string); ok {
					return account
				}
			}
		}
	}
	// Fallback: top-level worker_account (pow format)
	return GetString(v, "worker_account")
}
