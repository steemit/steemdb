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

// TestAccountNamePrefixFilterEscaping covers the regex escaping of user
// input in SearchAccounts (P1-9): the raw query reaches a $regex pattern,
// so every metacharacter must be quoted — an unescaped ".*" would degrade
// the prefix search into a full collection scan, and injected anchors or
// groups would change matching semantics.
func TestAccountNamePrefixFilterEscaping(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "plain prefix", query: "alice", want: "^alice"},
		{name: "dot-star would scan the whole collection", query: ".*", want: `^\.\*`},
		{name: "anchor injection", query: "^a$", want: `^\^a\$`},
		{name: "all metacharacters", query: `a.b*c+d?e(f)g[h]{i}j|k\l`, want: `^a\.b\*c\+d\?e\(f\)g\[h\]\{i\}j\|k\\l`},
		{name: "empty query stays an anchored empty pattern", query: "", want: "^"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := accountNamePrefixFilter(tt.query)
			cond, ok := filter["_id"].(bson.M)
			if !ok {
				t.Fatalf("filter[\"_id\"] is %T, want bson.M", filter["_id"])
			}
			pattern, _ := cond["$regex"].(string)
			if pattern != tt.want {
				t.Errorf("pattern = %q, want %q", pattern, tt.want)
			}
			if opts, _ := cond["$options"].(string); opts != "i" {
				t.Errorf("options = %q, want %q", opts, "i")
			}
		})
	}
}
