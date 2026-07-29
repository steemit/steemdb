package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

// CommentOptionsHandler processes "comment_options" operations
// → updates the "options" sub-document of the comment collection.
// Mirrors legacy sync.py:update_comment_options (line 375).
//
// _id format: "{author}/{permlink}" — matches the comment document.
type CommentOptionsHandler struct {
	inserter *MongoInserter
}

// NewCommentOptionsHandler creates a new CommentOptionsHandler.
func NewCommentOptionsHandler(inserter *MongoInserter) *CommentOptionsHandler {
	return &CommentOptionsHandler{inserter: inserter}
}

// Handle processes a comment_options operation.
// Stores the entire op_value under an "options" key on the comment document.
func (h *CommentOptionsHandler) Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error {
	v := op.OpValue
	author := GetString(v, "author")
	permlink := GetString(v, "permlink")

	if author == "" || permlink == "" {
		return fmt.Errorf("comment_options op missing author or permlink (id=%s)", op.ID)
	}

	id := author + "/" + permlink

	// Store the full op under "options", matching legacy sync.py:375-378.
	// Note: legacy does NOT transform any fields here — values are written raw.
	setFields := bson.M{
		"options": v,
	}

	return h.inserter.UpsertOneComplex(ctx, "comment", id, setFields, bson.M{
		"_id":      id,
		"author":   author,
		"permlink": permlink,
		"created":  blockTS,
	})
}
