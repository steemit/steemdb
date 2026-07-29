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
		currentBody := h.getCurrentBody(ctx, id)
		patched, ok := ApplySteemDiff(currentBody, body)
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

// getCurrentBody reads the current body field of a comment document.
// Returns "" if the document doesn't exist (first post or orphan diff).
func (h *CommentHandler) getCurrentBody(ctx context.Context, id string) string {
	var doc struct {
		Body string `bson:"body"`
	}
	col := h.inserter.db.Collection("comment")
	err := col.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		return ""
	}
	return doc.Body
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
