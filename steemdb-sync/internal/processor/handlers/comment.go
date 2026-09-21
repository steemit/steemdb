package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
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
		res := resolveDiffOp(current, op.ID, body)
		if res.skip {
			// The processor replays the last window after a crash that lands
			// between this write and the cursor advance. Re-applying a diff
			// would double-patch the body, so an applied diff is a no-op.
			// Non-diff upserts need no such guard: they $set the full body from
			// the op itself, making replay byte-identical.
			return nil
		}
		finalBody = res.body
		if !res.patched {
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
		"last_applied_op": op.ID, // idempotency marker — MUST stay in this $set: "comment" is unbuffered (single UpdateOne), so the marker and the patched body land atomically; a separate write would leave a half-updated guard state
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

// opTuple is the numeric (block, trx, op) coordinate parsed from an
// operation id ("{block}:{trx}:{op}", see model.OperationID).
type opTuple struct {
	block uint32
	trx   int64
	op    int64
}

// parseOpTuple parses an operation id into numeric coordinates. trx is an
// int64 because both ingest conventions fall outside int32: the plugin/live
// path normalizes virtual ops to trx=0xFFFFFFFF while repair uses trx=-1.
// Returns false when the id is not exactly three ':'-separated integers
// (legacy marker formats, corrupted documents).
func parseOpTuple(id string) (opTuple, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return opTuple{}, false
	}
	block, errBlock := strconv.ParseUint(parts[0], 10, 32)
	trx, errTrx := strconv.ParseInt(parts[1], 10, 64)
	op, errOp := strconv.ParseInt(parts[2], 10, 64)
	if errBlock != nil || errTrx != nil || errOp != nil {
		return opTuple{}, false
	}
	return opTuple{block: uint32(block), trx: trx, op: op}, true
}

// atOrBefore reports whether t <= u by numeric (block, trx, op) comparison.
func (t opTuple) atOrBefore(u opTuple) bool {
	if t.block != u.block {
		return t.block < u.block
	}
	if t.trx != u.trx {
		return t.trx < u.trx
	}
	return t.op <= u.op
}

// alreadyApplied reports whether a diff op was already persisted for this
// comment, so a window replay must not apply it again.
//
// The comparison is a numeric (block, trx, op) tuple compare, NOT a string
// compare: op ids are unpadded decimals that do not sort lexicographically
// ("100:10:0" < "100:2:0" as strings, yet 10 > 2 numerically). The op is
// already applied iff its tuple is <= the persisted marker's tuple, not merely
// equal: when one window holds two diffs for the same comment and the crash
// lands after the second write, replaying the older diff with an
// equality-only guard would double-patch the body (review finding sync F4).
// Since dispatch order is ascending (block_num, trx_index, op_index), the
// marker is monotonically non-decreasing within a window, which is what makes
// <= the correct skip condition.
//
// Fallback for unparseable ids: when either side fails to parse (legacy
// markers or corrupted documents), the guard degrades to exact string
// equality. It then skips only an op that is demonstrably identical to the
// marker — the pre-fix behavior — and never skips a distinct op based on
// unparsable data, so no new op-loss surface is introduced. An empty marker
// (document never written by the diff path) never skips.
func alreadyApplied(opID, lastAppliedOp string) bool {
	if lastAppliedOp == "" {
		return false
	}
	opT, okOp := parseOpTuple(opID)
	lastT, okLast := parseOpTuple(lastAppliedOp)
	if !okOp || !okLast {
		return lastAppliedOp == opID
	}
	return opT.atOrBefore(lastT)
}

// diffOutcome is the decision of the diff replay guard for one diff op.
type diffOutcome struct {
	// skip is true when the op was already applied and must be a no-op.
	skip bool
	// body is the body to persist: the patched body, or the raw diff body
	// when the patch could not be applied.
	body string
	// patched is false when the patch did not apply (orphan diff,
	// mismatched base) and the raw diff is stored with is_diff=true.
	patched bool
}

// resolveDiffOp evaluates the diff path of a comment op against the persisted
// comment state: the replay guard first, then the patch application. It is
// pure so window-replay sequences can be unit tested without a database.
func resolveDiffOp(current commentState, opID, diffBody string) diffOutcome {
	if alreadyApplied(opID, current.LastAppliedOp) {
		return diffOutcome{skip: true}
	}
	patched, ok := ApplySteemDiff(current.Body, diffBody)
	if !ok {
		return diffOutcome{body: diffBody, patched: false}
	}
	return diffOutcome{body: patched, patched: true}
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
// It routes through appendModel: the "comment" collection is unbuffered by
// design (diff reads and reward writebacks must see committed state), so this
// still executes immediately in batch mode.
func (m *MongoInserter) UpsertOneComplex(ctx context.Context, collection string, id interface{}, setFields bson.M, setOnInsertFields bson.M) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set":         setFields,
		"$setOnInsert": setOnInsertFields,
	}
	return m.appendModel(ctx, collection, filter, update)
}
