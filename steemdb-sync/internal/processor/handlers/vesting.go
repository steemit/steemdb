package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// VestingDepositHandler processes "transfer_to_vesting" operations
// → writes to the "vesting_deposit" collection.
// Mirrors legacy sync.py:save_vesting_deposit (line 159).
//
// _id format: "{blockid}/{from}/{to}"
type VestingDepositHandler struct {
	inserter *MongoInserter
}

// NewVestingDepositHandler creates a new VestingDepositHandler.
func NewVestingDepositHandler(inserter *MongoInserter) *VestingDepositHandler {
	return &VestingDepositHandler{inserter: inserter}
}

// Handle processes a transfer_to_vesting operation.
func (h *VestingDepositHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	from := GetString(v, "from")
	to := GetString(v, "to")

	id := fmt.Sprintf("%d/%s/%s", op.BlockNum, from, to)

	// Legacy stores only the numeric value (no symbol for vesting_deposit)
	amountVal := AssetValue(GetField(v, "amount"))

	doc := bson.M{
		"_id":    id,
		"_ts":    blockTS,
		"_block": op.BlockNum,
		"amount": amountVal,
	}
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "vesting_deposit", id, doc); err != nil {
		return fmt.Errorf("failed to upsert vesting_deposit %s: %w", id, err)
	}

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

// VestingWithdrawHandler processes "fill_vesting_withdraw" virtual operations
// → writes to the "vesting_withdraw" collection.
// Mirrors legacy sync.py:save_vesting_withdraw (line 172).
//
// _id format: "{blockid}/{from_account}/{to_account}"
type VestingWithdrawHandler struct {
	inserter *MongoInserter
}

// NewVestingWithdrawHandler creates a new VestingWithdrawHandler.
func NewVestingWithdrawHandler(inserter *MongoInserter) *VestingWithdrawHandler {
	return &VestingWithdrawHandler{inserter: inserter}
}

// Handle processes a fill_vesting_withdraw operation.
func (h *VestingWithdrawHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	fromAccount := GetString(v, "from_account")
	toAccount := GetString(v, "to_account")

	id := fmt.Sprintf("%d/%s/%s", op.BlockNum, fromAccount, toAccount)

	// Legacy parses deposited and withdrawn (value only, no symbol)
	depositedVal := AssetValue(GetField(v, "deposited"))
	withdrawnVal := AssetValue(GetField(v, "withdrawn"))

	doc := bson.M{
		"_id":       id,
		"_ts":       blockTS,
		"_block":    op.BlockNum,
		"deposited": depositedVal,
		"withdrawn": withdrawnVal,
	}
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "vesting_withdraw", id, doc); err != nil {
		return fmt.Errorf("failed to upsert vesting_withdraw %s: %w", id, err)
	}

	if err := h.inserter.QueueAccountDirty(ctx, fromAccount); err != nil {
		return fmt.Errorf("failed to queue account dirty (from=%s): %w", fromAccount, err)
	}
	if fromAccount != toAccount {
		if err := h.inserter.QueueAccountDirty(ctx, toAccount); err != nil {
			return fmt.Errorf("failed to queue account dirty (to=%s): %w", toAccount, err)
		}
	}

	return nil
}
