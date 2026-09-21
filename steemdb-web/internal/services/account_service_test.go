package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestAccountSummaryFromMapCommentCount covers the comment_count projection
// added for the accounts list. The refresher round-trips RPC values through
// JSON before upserting, so the count lands as a BSON double; other writers
// may emit int32/int64, and stub documents (dirty-marked but not yet
// refreshed) carry no count at all.
func TestAccountSummaryFromMapCommentCount(t *testing.T) {
	tests := []struct {
		name string
		doc  bson.M
		want int
	}{
		{
			name: "double (refresher JSON round-trip)",
			doc:  bson.M{"_id": "alice", "name": "alice", "comment_count": 4242.0},
			want: 4242,
		},
		{
			name: "int64",
			doc:  bson.M{"_id": "bob", "comment_count": int64(7)},
			want: 7,
		},
		{
			name: "int32",
			doc:  bson.M{"_id": "carol", "comment_count": int32(9)},
			want: 9,
		},
		{
			name: "absent on stub documents",
			doc:  bson.M{"_id": "dan"},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := accountSummaryFromMap(tt.doc)
			if summary.CommentCount != tt.want {
				t.Errorf("summary.CommentCount = %d, want %d", summary.CommentCount, tt.want)
			}
		})
	}
}
