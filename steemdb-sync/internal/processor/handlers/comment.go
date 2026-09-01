package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CommentHandler processes "comment" operations → writes to the "comment" collection.
//
// Phase 1 (this implementation): writes only the raw op fields (author, permlink, title,
// body, parent_*, json_metadata) — no get_content RPC call. Dynamic fields
// (active_votes, payouts, cashout_time, depth, net_votes) are left empty and will be
// filled by the comment_rescanner worker (Batch 5).
//
// _id format: "{author}/{permlink}" (matches legacy sync.py).
//
// Edit handling: ~20% of comment ops carry a diff body (starts with "@@ "). For these,
// the diff patch is applied to the previously-stored full body, producing an updated
// complete body. If the patch cannot be applied (e.g. orphan diff with no base text),
// the raw diff is stored with is_diff=true as fallback.
type CommentHandler struct {
	inserter *MongoInserter
}

// NewCommentHandler creates a new CommentHandler.
func NewCommentHandler(inserter *MongoInserter) *CommentHandler {
	return &CommentHandler{inserter: inserter}
}

// Handle processes a comment operation.
func (h *CommentHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	author := GetString(v, "author")
	permlink := GetString(v, "permlink")

	if author == "" || permlink == "" {
		return fmt.Errorf("comment op missing author or permlink (id=%s)", op.ID)
	}

	id := author + "/" + permlink
	body := GetString(v, "body")
	isDiff := IsDiffBody(body)

	// Apply diff patch if this is an edit (diff body).
	// Read the current body from the existing comment document, apply the patch,
	// and store the resulting full body.
	finalBody := body
	if isDiff {
		current := h.getCurrentState(ctx, id)
		if alreadyApplied(op.ID, current.LastAppliedOp) {
			// The processor replays the last block/window after a crash that
			// lands between this write and the cursor advance. Re-applying a
			// diff would double-patch the body, so an applied diff is a no-op.
			// Non-diff upserts need no such guard: they $set the full body from
			// the op itself, making replay byte-identical.
			return nil
		}
		patched, ok := ApplySteemDiff(current.Body, body)
		if ok {
			finalBody = patched
		} else {
			// Patch failed (e.g. orphan diff, mismatched base). Store raw diff + flag.
			log.Printf("[CommentHandler] Diff apply failed for %s (id=%s), storing raw diff", id, op.ID)
		}
	}

	// Parse json_metadata: legacy sync.py tries json.loads and falls back to the raw
	// string on failure. We do the same.
	var jsonMetadata interface{}
	jsonMetaStr := GetString(v, "json_metadata")
	if jsonMetaStr != "" {
		if err := json.Unmarshal([]byte(jsonMetaStr), &jsonMetadata); err != nil {
			// Keep raw string if not valid JSON (matches legacy behavior)
			jsonMetadata = jsonMetaStr
		}
	} else {
		jsonMetadata = map[string]interface{}{}
	}

	// $set: fields updated on every comment op (including edits)
	setFields := bson.M{
		"title":           GetString(v, "title"),
		"body":            finalBody,
		"is_diff":         isDiff && finalBody == body, // true only if patch failed and we stored raw diff
		"parent_author":   GetString(v, "parent_author"),
		"parent_permlink": GetString(v, "parent_permlink"),
		"json_metadata":   jsonMetadata,
		"last_update":     blockTS,
		"_ts":             blockTS,
		"_block":          op.BlockNum,
		"last_applied_op": op.ID, // idempotency marker, written atomically with the body
		"scanned":         time.Now(),
	}

	// $setOnInsert: fields written only when the document is first created
	setOnInsertFields := bson.M{
		"_id":       id,
		"author":    author,
		"permlink":  permlink,
		"created":   blockTS,
		"block_num": op.BlockNum,
	}

	return h.inserter.UpsertOneComplex(ctx, "comment", id, setFields, setOnInsertFields)
}

// commentState is the persisted state the diff path depends on.
type commentState struct {
	Body          string `bson:"body"`
	LastAppliedOp string `bson:"last_applied_op"`
}

// alreadyApplied reports whether a diff op was already persisted for this
// comment — i.e. the document's idempotency marker matches the op id exactly.
func alreadyApplied(opID, lastAppliedOp string) bool {
	return lastAppliedOp != "" && lastAppliedOp == opID
}

// getCurrentState reads the current body and idempotency marker of a comment
// document. Returns zero values if the document doesn't exist (first post or
// orphan diff).
func (h *CommentHandler) getCurrentState(ctx context.Context, id string) commentState {
	var state commentState
	col := h.inserter.db.Collection("comment")
	if err := col.FindOne(ctx, bson.M{"_id": id}).Decode(&state); err != nil {
		return commentState{}
	}
	return state
}

// UpsertOneComplex performs an upsert with separate $set and $setOnInsert stages.
// This allows "created" to be set only on first insert while "last_update" updates every time.
func (m *MongoInserter) UpsertOneComplex(ctx context.Context, collection string, id interface{}, setFields bson.M, setOnInsertFields bson.M) error {
	col := m.db.Collection(collection)
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set":         setFields,
		"$setOnInsert": setOnInsertFields,
	}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
}
