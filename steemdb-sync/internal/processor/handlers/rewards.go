package handlers

import (
	"context"
	"fmt"
	"log"
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

	rewardVal := AssetValue(GetField(v, "reward"))

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

	sbdPayout := AssetValue(GetField(v, "sbd_payout"))
	steemPayout := AssetValue(GetField(v, "steem_payout"))
	vestingPayout := AssetValue(GetField(v, "vesting_payout"))

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

	// Write reward back to the comment document (legacy sync.py:156).
	// The comment may not exist yet (it will be created by the comment handler later),
	// so MatchedCount==0 is expected and harmless. Only log on actual errors.
	commentID := author + "/" + permlink
	matched, err := h.inserter.UpdateOne(ctx, "comment",
		bson.M{"_id": commentID},
		bson.M{"$set": bson.M{"reward": doc}},
	)
	if err != nil {
		// Non-fatal: don't fail the operation, but log the real error.
		log.Printf("[AuthorReward] Failed to write reward back to comment %s: %v", commentID, err)
	} else if !matched {
		// Normal during cold start: comment doc not yet created. No-op.
	}

	// Queue author account for refresh
	if err := h.inserter.QueueAccountDirty(ctx, author); err != nil {
		return fmt.Errorf("failed to queue account dirty (author=%s): %w", author, err)
	}

	return nil
}

// BenefactorRewardHandler processes "comment_benefactor_reward" virtual operations
// → writes to the "benefactor_reward" collection.
// Mirrors legacy sync.py:save_benefactor_reward (line 227).
//
// Uses Pattern B: multi-field query filter (no _id).
// filter: {_block, benefactor, permlink, author}
type BenefactorRewardHandler struct {
	inserter *MongoInserter
}

// NewBenefactorRewardHandler creates a new BenefactorRewardHandler.
func NewBenefactorRewardHandler(inserter *MongoInserter) *BenefactorRewardHandler {
	return &BenefactorRewardHandler{inserter: inserter}
}

// Handle processes a comment_benefactor_reward operation.
func (h *BenefactorRewardHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	benefactor := GetString(v, "benefactor")
	permlink := GetString(v, "permlink")
	author := GetString(v, "author")

	// Legacy parses vesting_payout and stores it as "reward"
	rewardVal := AssetValue(GetField(v, "vesting_payout"))

	filter := bson.M{
		"_block":     op.BlockNum,
		"benefactor": benefactor,
		"permlink":   permlink,
		"author":     author,
	}

	doc := bson.M{
		"_block":     op.BlockNum,
		"_ts":        blockTS,
		"benefactor": benefactor,
		"permlink":   permlink,
		"author":     author,
		"reward":     rewardVal,
	}
	// Preserve other fields from op_value (e.g. original vesting_payout string)
	for k, val := range v {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOneByFilter(ctx, "benefactor_reward", filter, doc); err != nil {
		return fmt.Errorf("failed to upsert benefactor_reward: %w", err)
	}

	// Legacy does NOT queue_update_account for benefactor_reward
	return nil
}
