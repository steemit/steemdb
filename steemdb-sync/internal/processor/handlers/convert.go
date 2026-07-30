package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// ConvertHandler processes "convert" operations → writes to the "convert" collection.
// Mirrors legacy sync.py:save_convert (line 97).
//
// _id format: "{blockid}/{requestid}"
type ConvertHandler struct {
	inserter *MongoInserter
}

// NewConvertHandler creates a new ConvertHandler.
func NewConvertHandler(inserter *MongoInserter) *ConvertHandler {
	return &ConvertHandler{inserter: inserter}
}

// Handle processes a convert operation.
func (h *ConvertHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	owner := GetString(v, "owner")

	// requestid may be numeric or string depending on source
	requestID := fmt.Sprintf("%v", v["requestid"])

	id := fmt.Sprintf("%d/%s", op.BlockNum, requestID)

	amountVal, amountType := ParseAsset(GetField(v, "amount"))

	doc := bson.M{
		"_id":    id,
		"_ts":    blockTS,
		"_block": op.BlockNum,
		"amount": amountVal,
		"type":   amountType,
	}
	// Preserve other fields (owner, requestid, etc.)
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "convert", id, doc); err != nil {
		return fmt.Errorf("failed to upsert convert %s: %w", id, err)
	}

	// Queue owner account for refresh
	if err := h.inserter.QueueAccountDirty(ctx, owner); err != nil {
		return fmt.Errorf("failed to queue account dirty (owner=%s): %w", owner, err)
	}

	return nil
}
