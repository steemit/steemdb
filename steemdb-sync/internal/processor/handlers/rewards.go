package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// CurationRewardHandler processes "curation_reward" virtual operations
// → writes to the "curation_reward" collection.
// Mirrors legacy sync.py:save_curation_reward (line 123).
//
// _id format: "{blockid}/{curator}/{comment_author}/{comment_permlink}"
type CurationRewardHandler struct {
	inserter *MongoInserter
}

// NewCurationRewardHandler creates a new CurationRewardHandler.
func NewCurationRewardHandler(inserter *MongoInserter) *CurationRewardHandler {
	return &CurationRewardHandler{inserter: inserter}
}

// Handle processes a curation_reward operation.
func (h *CurationRewardHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	curator := GetString(v, "curator")
	commentAuthor := GetString(v, "comment_author")
	commentPermlink := GetString(v, "comment_permlink")
	rewardStr := GetString(v, "reward")

	rewardVal := AmountValue(rewardStr)

	id := fmt.Sprintf("%d/%s/%s/%s", op.BlockNum, curator, commentAuthor, commentPermlink)

	doc := bson.M{
		"_id":             id,
		"_ts":             blockTS,
		"_block":          op.BlockNum,
		"curator":         curator,
		"comment_author":  commentAuthor,
		"comment_permlink": commentPermlink,
		"reward":          rewardVal,
	}
	// Preserve other fields (e.g. legacy stores the raw reward string too)
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "curation_reward", id, doc); err != nil {
		return fmt.Errorf("failed to upsert curation_reward %s: %w", id, err)
	}

	// Queue curator account for refresh
	if err := h.inserter.QueueAccountDirty(ctx, curator); err != nil {
		return fmt.Errorf("failed to queue account dirty (curator=%s): %w", curator, err)
	}

	return nil
}

// AuthorRewardHandler processes "author_reward" virtual operations
// → writes to the "author_reward" collection + updates comment.reward.
// Mirrors legacy sync.py:save_author_reward (line 134).
//
// Note: unlike legacy, Batch 1 does NOT call update_comment() here (that requires
// the comment handler's get_content RPC, added in Batch 2). For now it only writes
// the author_reward document. The app_name/app_version extraction from comment.json_metadata
// will be added when the comment handler is available.
//
// _id format: "{blockid}/{author}/{permlink}"
type AuthorRewardHandler struct {
	inserter *MongoInserter
}

// NewAuthorRewardHandler creates a new AuthorRewardHandler.
func NewAuthorRewardHandler(inserter *MongoInserter) *AuthorRewardHandler {
	return &AuthorRewardHandler{inserter: inserter}
}

// Handle processes an author_reward operation.
func (h *AuthorRewardHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	author := GetString(v, "author")
	permlink := GetString(v, "permlink")

	sbdPayout := AmountValue(GetString(v, "sbd_payout"))
	steemPayout := AmountValue(GetString(v, "steem_payout"))
	vestingPayout := AmountValue(GetString(v, "vesting_payout"))

	id := fmt.Sprintf("%d/%s/%s", op.BlockNum, author, permlink)

	doc := bson.M{
		"_id":            id,
		"_ts":            blockTS,
		"_block":         op.BlockNum,
		"author":         author,
		"permlink":       permlink,
		"sbd_payout":     sbdPayout,
		"steem_payout":   steemPayout,
		"vesting_payout": vestingPayout,
	}
	// Preserve other fields from op_value
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOne(ctx, "author_reward", id, doc); err != nil {
		return fmt.Errorf("failed to upsert author_reward %s: %w", id, err)
	}

	// Write reward back to the comment document (legacy sync.py:156)
	commentID := author + "/" + permlink
	col := h.inserter.db.Collection("comment")
	_, err := col.UpdateOne(ctx, bson.M{"_id": commentID}, bson.M{"$set": bson.M{"reward": doc}})
	if err != nil {
		// Non-fatal: comment may not exist yet (will be created by comment handler later)
		// Log but don't fail the operation.
		_ = err
	}

	// Queue author account for refresh
	if err := h.inserter.QueueAccountDirty(ctx, author); err != nil {
		return fmt.Errorf("failed to queue account dirty (author=%s): %w", author, err)
	}

	return nil
}
