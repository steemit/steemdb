package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
)

// AccountCreateHandler processes the three account-creation ops:
// "account_create", "account_create_with_delegation" (HF17+) and
// "create_claimed_account" (HF20+).
//
// Per docs/rules/account-doc-creation.md these ops (plus pow/pow2 mining and
// the genesis accounts) are the ONLY source of new accounts on chain, so the
// handler upserts the new account stub with _dirty (QueueAccountCreate) for
// the AccountRefresher to fill, and dirty-marks the creator (fee/delegation
// changed its balances). It writes no derived collection.
//
// Write-ordering classification: marks coalesce in the batcher maps to one
// update per account per window — replay cannot double-apply.
type AccountCreateHandler struct {
	inserter *MongoInserter
}

// NewAccountCreateHandler creates a new AccountCreateHandler.
func NewAccountCreateHandler(inserter *MongoInserter) *AccountCreateHandler {
	return &AccountCreateHandler{inserter: inserter}
}

// Handle processes an account-creation operation.
func (h *AccountCreateHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	newAccount := GetString(v, "new_account_name")
	creator := GetString(v, "creator")

	if err := h.inserter.QueueAccountCreate(ctx, newAccount); err != nil {
		return fmt.Errorf("failed to queue account create (new_account_name=%s): %w", newAccount, err)
	}
	if creator != "" && creator != newAccount {
		if err := h.inserter.QueueAccountDirty(ctx, creator); err != nil {
			return fmt.Errorf("failed to queue account dirty (creator=%s): %w", creator, err)
		}
	}

	return nil
}
