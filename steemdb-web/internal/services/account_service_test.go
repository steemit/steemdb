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

// TestAccountSortField covers the sort-key whitelist of the accounts
// listing: every accepted key maps to an index-backed account field (sync's
// indexInventory: reputation, vesting_shares; _id is the account name with
// the default index), and anything else falls back to the default instead
// of producing an unindexed collection sort.
func TestAccountSortField(t *testing.T) {
	tests := []struct {
		name   string
		sortBy string
		want   string
	}{
		{name: "name maps to _id (the account name on every document)", sortBy: "name", want: "_id"},
		{name: "reputation", sortBy: "reputation", want: "reputation"},
		{name: "vests alias", sortBy: "vests", want: "vesting_shares"},
		{name: "vesting_shares canonical", sortBy: "vesting_shares", want: "vesting_shares"},
		{name: "empty falls back to default", sortBy: "", want: "reputation"},
		{name: "unindexed balance is not offered and falls back", sortBy: "balance", want: "reputation"},
		{name: "unknown field falls back", sortBy: "post_count", want: "reputation"},
		{name: "injection attempt falls back", sortBy: "reputation; drop", want: "reputation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountSortField(tt.sortBy); got != tt.want {
				t.Errorf("accountSortField(%q) = %q, want %q", tt.sortBy, got, tt.want)
			}
		})
	}
}

// TestAccountListFilter covers the accounts listing search wiring: an empty
// search lists everything, a non-empty search reuses the escaped name-prefix
// matcher (so the listing and /accounts/search agree on both the field (_id)
// and the regex escaping).
func TestAccountListFilter(t *testing.T) {
	t.Run("empty search lists everything", func(t *testing.T) {
		filter := accountListFilter("")
		if len(filter) != 0 {
			t.Errorf("filter = %v, want empty", filter)
		}
	})

	t.Run("non-empty search is the escaped _id prefix filter", func(t *testing.T) {
		filter := accountListFilter("a.*")
		cond, ok := filter["_id"].(bson.M)
		if !ok {
			t.Fatalf("filter[\"_id\"] is %T, want bson.M", filter["_id"])
		}
		pattern, _ := cond["$regex"].(string)
		if pattern != `^a\.\*` {
			t.Errorf("pattern = %q, want %q", pattern, `^a\.\*`)
		}
		if len(filter) != 1 {
			t.Errorf("filter = %v, want only the _id condition", filter)
		}
	})
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
