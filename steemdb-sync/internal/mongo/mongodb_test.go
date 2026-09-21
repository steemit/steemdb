package mongo

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// expectedIndexes is the golden inventory. It exists so that every index
// addition/removal is a deliberate, reviewed change: the inventory is the
// single index authority for the whole database (see indexInventory), and an
// accidental drift between a handler filter or a web query and this list is
// exactly the class of bug (Pattern-B COLLSCAN, phantom web indexes) this
// file guards against.
var expectedIndexes = []struct {
	collection string
	keys       bson.D
	partial    interface{} // expected partialFilterExpression, nil for plain indexes
}{
	// Raw layer
	{"blocks", bson.D{{Key: "timestamp", Value: 1}}, nil},
	{"transactions", bson.D{{Key: "block_num", Value: 1}}, nil},
	{"operations", bson.D{{Key: "block_num", Value: 1}}, nil},
	{"operations", bson.D{{Key: "trx_id", Value: 1}}, nil},
	{"operations", bson.D{{Key: "op_type", Value: 1}}, nil},
	{"operations", bson.D{{Key: "virtual", Value: 1}}, nil},
	{"operations", bson.D{{Key: "accounts", Value: 1}, {Key: "block_num", Value: -1}}, nil},
	// Pattern-B write-path indexes (must lead with the handler filter)
	{"follow", bson.D{{Key: "_block", Value: 1}, {Key: "follower", Value: 1}, {Key: "following", Value: 1}}, nil},
	{"reblog", bson.D{{Key: "_block", Value: 1}, {Key: "permlink", Value: 1}, {Key: "account", Value: 1}}, nil},
	{"witness_vote", bson.D{{Key: "_ts", Value: 1}, {Key: "account", Value: 1}, {Key: "witness", Value: 1}}, nil},
	{"benefactor_reward", bson.D{{Key: "_ts", Value: 1}, {Key: "benefactor", Value: 1}, {Key: "permlink", Value: 1}, {Key: "author", Value: 1}}, nil},
	{"benefactor_reward", bson.D{{Key: "_block", Value: 1}, {Key: "benefactor", Value: 1}, {Key: "permlink", Value: 1}, {Key: "author", Value: 1}}, nil},
	// Read-side: labs time-range aggregations on _ts
	{"vesting_deposit", bson.D{{Key: "_ts", Value: 1}}, nil},
	{"vesting_withdraw", bson.D{{Key: "_ts", Value: 1}}, nil},
	{"curation_reward", bson.D{{Key: "_ts", Value: 1}}, nil},
	{"author_reward", bson.D{{Key: "_ts", Value: 1}}, nil},
	// vote / reblog per-post listings
	{"vote", bson.D{{Key: "author", Value: 1}, {Key: "permlink", Value: 1}, {Key: "_ts", Value: 1}}, nil},
	{"vote", bson.D{{Key: "weight", Value: 1}}, bson.M{"weight": bson.M{"$lt": 0}}},
	{"reblog", bson.D{{Key: "author", Value: 1}, {Key: "permlink", Value: 1}, {Key: "_ts", Value: 1}}, nil},
	// account listings, labs joins and power-down schedule
	{"account", bson.D{{Key: "name", Value: 1}}, nil},
	{"account", bson.D{{Key: "reputation", Value: -1}}, nil},
	{"account", bson.D{{Key: "vesting_shares", Value: -1}}, nil},
	{"account", bson.D{{Key: "next_vesting_withdrawal", Value: 1}}, nil},
	// witness snapshot
	{"witness", bson.D{{Key: "owner", Value: 1}}, nil},
	{"witness", bson.D{{Key: "votes", Value: -1}}, nil},
	{"witness", bson.D{{Key: "total_missed", Value: 1}}, nil},
	// comment: posts list, replies, rescanner queues, sort columns
	{"comment", bson.D{{Key: "depth", Value: 1}, {Key: "created", Value: -1}}, nil},
	{"comment", bson.D{{Key: "parent_author", Value: 1}, {Key: "parent_permlink", Value: 1}, {Key: "created", Value: -1}}, nil},
	{"comment", bson.D{{Key: "scanned", Value: 1}, {Key: "created", Value: 1}}, nil},
	{"comment", bson.D{{Key: "depth", Value: 1}, {Key: "pending_payout_value", Value: 1}}, nil},
	{"comment", bson.D{{Key: "category", Value: 1}}, nil},
	{"comment", bson.D{{Key: "created", Value: -1}}, nil},
	{"comment", bson.D{{Key: "net_votes", Value: -1}}, nil},
	{"comment", bson.D{{Key: "pending_payout_value", Value: -1}}, nil},
	{"comment", bson.D{{Key: "total_payout_value", Value: -1}}, nil},
}

func TestIndexInventory(t *testing.T) {
	specs := indexInventory()

	if len(specs) != len(expectedIndexes) {
		t.Fatalf("index inventory has %d entries, expected %d — if this change is intentional, update expectedIndexes with a justification", len(specs), len(expectedIndexes))
	}

	for i, want := range expectedIndexes {
		got := specs[i]
		if got.collection != want.collection {
			t.Errorf("entry %d: collection = %q, expected %q", i, got.collection, want.collection)
			continue
		}
		if fmt.Sprint(got.model.Keys) != fmt.Sprint(want.keys) {
			t.Errorf("%s entry %d: keys = %v, expected %v", want.collection, i, got.model.Keys, want.keys)
		}
		var gotPartial interface{}
		if got.model.Options != nil {
			gotPartial = got.model.Options.PartialFilterExpression
		}
		if fmt.Sprint(gotPartial) != fmt.Sprint(want.partial) {
			t.Errorf("%s %v: partialFilterExpression = %v, expected %v", want.collection, want.keys, gotPartial, want.partial)
		}
	}
}

// TestLegacyWebIndexesAreNotRecreated pins the cleanup side of the index
// authority: every legacy web-built index name sync drops must not appear as
// (a prefix of) an inventory index with the same collection+keys, or the
// startup would drop and rebuild it forever.
func TestLegacyWebIndexesAreNotRecreated(t *testing.T) {
	specs := indexInventory()
	for coll, names := range legacyWebIndexes {
		for _, name := range names {
			for _, s := range specs {
				if s.collection != coll {
					continue
				}
				if keys, ok := s.model.Keys.(bson.D); ok && defaultIndexName(keys) == name {
					t.Errorf("inventory recreates legacy web index %s.%s that dropLegacyWebIndexes removes", coll, name)
				}
			}
		}
	}
}

// defaultIndexName mimics the server's default index naming (field_dir
// fields joined by "_", e.g. {a:1,b:-1} -> "a_1_b_-1"), which is what both
// the old web builder and sync's inventory rely on.
func defaultIndexName(keys bson.D) string {
	name := ""
	for _, k := range keys {
		if name != "" {
			name += "_"
		}
		name += fmt.Sprintf("%s_%v", k.Key, k.Value)
	}
	return name
}

// TestPatternBFiltersIndexed pins the write-side coupling: every Pattern-B
// handler upserts by a business-field filter, and that filter must be the
// leading prefix of an index in the inventory or every upsert is a collection
// scan (the benefactor_reward incident: flush 6203ms -> 98ms after the index).
func TestPatternBFiltersIndexed(t *testing.T) {
	filters := []struct {
		collection string
		filter     []string
	}{
		{"follow", []string{"_block", "follower", "following"}},
		{"reblog", []string{"_block", "permlink", "account"}},
		{"witness_vote", []string{"_ts", "account", "witness"}},
		{"benefactor_reward", []string{"_block", "benefactor", "permlink", "author"}},
	}

	specs := indexInventory()
	for _, f := range filters {
		covered := false
		for _, s := range specs {
			if s.collection != f.collection {
				continue
			}
			keys, ok := s.model.Keys.(bson.D)
			if !ok || len(keys) < len(f.filter) {
				continue
			}
			match := true
			for i, field := range f.filter {
				if keys[i].Key != field {
					match = false
					break
				}
			}
			if match {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("Pattern-B filter {%v} on %q is not the prefix of any index — upserts will collection-scan", f.filter, f.collection)
		}
	}
}
