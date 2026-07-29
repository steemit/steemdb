package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
)

// makeCommentOp builds a comment Operation for testing.
func makeCommentOp(blockNum uint32, v map[string]interface{}) *model.Operation {
	return &model.Operation{
		ID:       fmt.Sprintf("%d:0:0", blockNum),
		BlockNum: blockNum,
		OpType:   "comment",
		OpValue:  v,
	}
}

// TestCommentHandler_MissingAuthor checks error handling for malformed ops.
func TestCommentHandler_MissingAuthor(t *testing.T) {
	h := &CommentHandler{inserter: &MongoInserter{}}

	op := makeCommentOp(100, map[string]interface{}{
		"permlink": "test", // missing author
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing author, got nil")
	}
	if !strings.Contains(err.Error(), "missing author") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCommentHandler_MissingPermlink checks error handling for missing permlink.
func TestCommentHandler_MissingPermlink(t *testing.T) {
	h := &CommentHandler{inserter: &MongoInserter{}}

	op := makeCommentOp(100, map[string]interface{}{
		"author": "alice", // missing permlink
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing permlink, got nil")
	}
	if !strings.Contains(err.Error(), "missing author") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCommentOptionsHandler_MissingFields checks error for missing author/permlink.
func TestCommentOptionsHandler_MissingFields(t *testing.T) {
	h := &CommentOptionsHandler{inserter: &MongoInserter{}}

	op := &model.Operation{
		ID:       "200:0:0",
		BlockNum: 200,
		OpType:   "comment_options",
		OpValue:  map[string]interface{}{"author": "alice"}, // missing permlink
	}

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing permlink, got nil")
	}
}

// TestParseJsonMetadata_logic verifies the json_metadata parsing behavior
// that the comment handler relies on.
func TestParseJsonMetadata_logic(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid json", `{"tags":["test"],"app":"steemit/1.0"}`, false},
		{"empty object", `{}`, false},
		{"empty string", "", true},
		{"invalid json", "not json {{{", true},
		{"trailing comma", `{"tags":["a"],}`, true},
		{"nested object", `{"tags":["a","b"],"users":["alice"],"app":{"name":"x","ver":"1"}}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			err := json.Unmarshal([]byte(tt.input), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
