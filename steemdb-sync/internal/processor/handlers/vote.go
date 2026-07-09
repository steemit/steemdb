package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// VoteHandler processes "vote" operations → writes to the "vote" collection.
// Mirrors legacy sync.py:save_vote (line 279).
//
// _id format: "{blockid}/{voter}/{author}/{permlink}"
type VoteHandler struct {
	inserter *MongoInserter
}

// NewVoteHandler creates a new VoteHandler.
func NewVoteHandler(inserter *MongoInserter) *VoteHandler {
	return &VoteHandler{inserter: inserter}
}

// Handle processes a vote operation.
func (h *VoteHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	voter := GetString(v, "voter")
	author := GetString(v, "author")
	permlink := GetString(v, "permlink")

	id := fmt.Sprintf("%d/%s/%s/%s", op.BlockNum, voter, author, permlink)

	// Build the document: full op value + _id + _ts (matches legacy behavior)
	doc := bson.M{
		"_id":     id,
		"_ts":     blockTS,
		"_block":  op.BlockNum,
		"voter":   voter,
		"author":  author,
		"permlink": permlink,
		"weight":  v["weight"],
	}
	// Preserve any extra fields from op_value that legacy stored (e.g. some forks add fields)
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "vote", id, doc); err != nil {
		return fmt.Errorf("failed to upsert vote %s: %w", id, err)
	}

	return nil
}
