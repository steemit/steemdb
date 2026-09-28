package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// CustomJSONHandler processes "custom_json" operations.
// It parses the inner JSON to determine the action type:
//   - data[0] == "reblog" → writes to "reblog" collection
//   - data[0] == "follow" → writes to "follow" collection
//   - anything else → silently skipped
//
// Mirrors legacy sync.py:save_custom_json (line 186) + save_reblog (line 242) +
// save_follow (line 210).
type CustomJSONHandler struct {
	inserter *MongoInserter
}

// NewCustomJSONHandler creates a new CustomJSONHandler.
func NewCustomJSONHandler(inserter *MongoInserter) *CustomJSONHandler {
	return &CustomJSONHandler{inserter: inserter}
}

// Handle processes a custom_json operation.
func (h *CustomJSONHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue

	// Parse the "json" field — may be a string (needs unmarshal) or already decoded
	var data []interface{}
	jsonRaw := v["json"]

	switch j := jsonRaw.(type) {
	case string:
		if err := json.Unmarshal([]byte(j), &data); err != nil {
			// Legacy silently skips on parse error
			return nil
		}
	case []interface{}:
		data = j
	default:
		return nil
	}

	if len(data) == 0 {
		return nil
	}

	action, ok := data[0].(string)
	if !ok {
		return nil
	}

	switch action {
	case "reblog":
		return h.handleReblog(ctx, op, data, blockTS)
	case "follow":
		return h.handleFollow(ctx, op, data, blockTS)
	default:
		// Unknown custom_json action — silently skip
		return nil
	}
}

// handleReblog processes the reblog custom_json action.
// Mirrors legacy sync.py:save_reblog (line 242).
// Requires payload (data[1]) to contain both "permlink" and "account".
func (h *CustomJSONHandler) handleReblog(ctx context.Context, op *model.Operation, data []interface{}, blockTS time.Time) error {
	if len(data) < 2 {
		return nil
	}

	payload, ok := data[1].(map[string]interface{})
	if !ok {
		return nil
	}

	account := GetString(payload, "account")
	permlink := GetString(payload, "permlink")

	// Legacy requires BOTH permlink and account
	if permlink == "" || account == "" {
		return nil
	}

	filter := bson.M{
		"_block":   op.BlockNum,
		"permlink": permlink,
		"account":  account,
	}

	doc := bson.M{
		"_block":   op.BlockNum,
		"_ts":      blockTS,
		"permlink": permlink,
		"account":  account,
	}
	// Preserve other fields from payload (e.g. author)
	for k, val := range payload {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOneByFilter(ctx, "reblog", filter, doc); err != nil {
		return fmt.Errorf("failed to upsert reblog: %w", err)
	}

	// Legacy does NOT queue_update_account for reblog
	return nil
}

// handleFollow processes the follow custom_json action.
// Mirrors legacy sync.py:save_follow (line 210).
// Requires payload (data[1]) to contain both "follower" and "following".
func (h *CustomJSONHandler) handleFollow(ctx context.Context, op *model.Operation, data []interface{}, blockTS time.Time) error {
	if len(data) < 2 {
		return nil
	}

	payload, ok := data[1].(map[string]interface{})
	if !ok {
		return nil
	}

	follower := GetString(payload, "follower")
	following := GetString(payload, "following")

	// Legacy requires BOTH follower and following
	if follower == "" || following == "" {
		return nil
	}

	filter := bson.M{
		"_block":    op.BlockNum,
		"follower":  follower,
		"following": following,
	}

	doc := bson.M{
		"_block":    op.BlockNum,
		"_ts":       blockTS,
		"follower":  follower,
		"following": following,
	}
	// Preserve other fields from payload (e.g. what)
	for k, val := range payload {
		if _, exists := doc[k]; !exists {
			doc[k] = val
		}
	}

	if err := h.inserter.UpsertOneByFilter(ctx, "follow", filter, doc); err != nil {
		return fmt.Errorf("failed to upsert follow: %w", err)
	}

	// No account dirty marks here. Follow/unfollow changes no field of the
	// chain account_object (follower counts live in the follow_api/hivemind
	// domain, not in get_accounts), and the payload's following field is a
	// user-controlled string the chain does not validate as an account —
	// dirty-marking it was the sole source of phantom account stubs
	// (docs/rules/account-doc-creation.md). The raw event record above is
	// kept; current-state follow statistics belong to hivemind.

	return nil
}
