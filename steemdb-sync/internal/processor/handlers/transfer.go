package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// TransferHandler processes "transfer" operations → writes to the "transfer" collection.
// Mirrors legacy sync.py:save_transfer (line 109).
//
// _id format: "{blockid}/{from}/{to}"
type TransferHandler struct {
	inserter *MongoInserter
}

// NewTransferHandler creates a new TransferHandler.
func NewTransferHandler(inserter *MongoInserter) *TransferHandler {
	return &TransferHandler{inserter: inserter}
}

// Handle processes a transfer operation.
func (h *TransferHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	from := GetString(v, "from")
	to := GetString(v, "to")

	amountVal, amountType := ParseAsset(GetField(v, "amount"))

	id := fmt.Sprintf("%d/%s/%s", op.BlockNum, from, to)

	doc := bson.M{
		"_id":    id,
		"_ts":    blockTS,
		"_block": op.BlockNum,
		"from":   from,
		"to":     to,
		"amount": amountVal,
		"type":   amountType,
	}
	// Preserve memo and any other fields from op_value
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "transfer", id, doc); err != nil {
		return fmt.Errorf("failed to upsert transfer %s: %w", id, err)
	}

	// Queue account dirty refresh for both parties
	if err := h.inserter.QueueAccountDirty(ctx, from); err != nil {
		return fmt.Errorf("failed to queue account dirty (from=%s): %w", from, err)
	}
	if from != to {
		if err := h.inserter.QueueAccountDirty(ctx, to); err != nil {
			return fmt.Errorf("failed to queue account dirty (to=%s): %w", to, err)
		}
	}

	return nil
}
