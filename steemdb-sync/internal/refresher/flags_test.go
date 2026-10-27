package refresher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestBuildFlagSnapshotEntry guards the fold from raw aggregation rows to the
// snapshot row: distinct voter/post counts plus the top-N voters by op count.
// Voters is one entry per downvote op, so repeats are expected.
func TestBuildFlagSnapshotEntry(t *testing.T) {
	row := flagVoteRow{
		Author: "appics-introduce",
		// 6 ops from 3 distinct voters; steemcleaners flagged 3 times.
		Voters: []string{"steemcleaners", "steemcleaners", "steemcleaners", "spaminator", "spaminator", "bob"},
		Posts:  []string{"p1", "p2", "p3"},
	}
	row.Count = len(row.Voters)

	got := buildFlagSnapshotEntry(row)

	if got.Author != "appics-introduce" {
		t.Errorf("Author = %q, want appics-introduce", got.Author)
	}
	if got.Count != 6 {
		t.Errorf("Count = %d, want 6 (op count)", got.Count)
	}
	if got.VoterCount != 3 {
		t.Errorf("VoterCount = %d, want 3 (distinct voters)", got.VoterCount)
	}
	if got.PostCount != 3 {
		t.Errorf("PostCount = %d, want 3 (distinct posts)", got.PostCount)
	}
	if len(got.Voters) != 3 {
		t.Fatalf("Voters length = %d, want 3", len(got.Voters))
	}
	if got.Voters[0] != (flagVoterCount{Voter: "steemcleaners", Count: 3}) {
		t.Errorf("Voters[0] = %+v, want steemcleaners/3", got.Voters[0])
	}
	if got.Voters[1] != (flagVoterCount{Voter: "spaminator", Count: 2}) {
		t.Errorf("Voters[1] = %+v, want spaminator/2", got.Voters[1])
	}
	if got.Voters[2] != (flagVoterCount{Voter: "bob", Count: 1}) {
		t.Errorf("Voters[2] = %+v, want bob/1", got.Voters[2])
	}
}

// TestBuildFlagSnapshotEntryTopVoters checks the cap and the deterministic
// tie-break (equal op counts are ordered by name, so a re-run of the same
// snapshot yields the same document).
func TestBuildFlagSnapshotEntryTopVoters(t *testing.T) {
	voters := make([]string, 0, 20)
	for _, name := range []string{"zeta", "beta", "yankee", "alpha", "xray", "delta",
		"whiskey", "echo", "victor", "foxtrot", "uniform", "golf"} {
		voters = append(voters, name, name)
	}

	got := buildFlagSnapshotEntry(flagVoteRow{Author: "a", Count: 24, Voters: voters, Posts: nil})

	if len(got.Voters) != flagsTopVoters {
		t.Fatalf("Voters length = %d, want %d", len(got.Voters), flagsTopVoters)
	}
	if got.VoterCount != 12 {
		t.Errorf("VoterCount = %d, want 12 (all distinct voters, not just the top N)", got.VoterCount)
	}
	// All counts are 2, so alphabetical order decides.
	want := []string{"alpha", "beta", "delta", "echo", "foxtrot", "golf", "uniform", "victor", "whiskey", "xray"}
	for i, name := range want {
		if got.Voters[i].Voter != name {
			t.Errorf("Voters[%d] = %q, want %q (ties break by name)", i, got.Voters[i].Voter, name)
		}
	}
}

// TestFlagsTickerAgainstMongo runs the refresher tick against a real MongoDB:
// the aggregation, the catch-up gate and the status write are exercised
// together (unit tests can pin the fold, not the pipeline).
func TestFlagsTickerAgainstMongo(t *testing.T) {
	uri := mongoTestURI()
	if uri == "" {
		t.Skip("no test mongo configured")
	}

	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("steemdb_refresher_flags_test")
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("drop: %v", err)
	}

	// Open the catch-up gate: processor cursor at 1000 with the blocks
	// frontier at the same height.
	if _, err := db.Collection("status").InsertOne(ctx, bson.M{"_id": "processor_height", "value": int64(1000)}); err != nil {
		t.Fatalf("seed processor_height: %v", err)
	}
	if _, err := db.Collection("blocks").InsertOne(ctx, bson.M{"_id": int64(1000)}); err != nil {
		t.Fatalf("seed block: %v", err)
	}

	// weight<0 vote op log. "spammer" is flagged 4 times by 2 accounts across
	// 2 posts; "troll" 2 times by 1 account; the weight>0 vote must not count.
	votes := []bson.M{
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "alice", "author": "spammer", "permlink": "p1", "weight": int64(-10000)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "alice", "author": "spammer", "permlink": "p1", "weight": int64(-10000)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "bob", "author": "spammer", "permlink": "p1", "weight": int64(-500)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "bob", "author": "spammer", "permlink": "p2", "weight": int64(-500)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "carol", "author": "troll", "permlink": "p3", "weight": int64(-10000)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "carol", "author": "troll", "permlink": "p3", "weight": int64(-10000)},
		{"_ts": time.Now().UTC(), "_block": int64(999), "voter": "dave", "author": "upvoted", "permlink": "p4", "weight": int64(10000)},
	}
	for i, vote := range votes {
		vote["_id"] = fmt.Sprintf("999/%s/%d", vote["voter"], i)
		if _, err := db.Collection("vote").InsertOne(ctx, vote); err != nil {
			t.Fatalf("seed vote %d: %v", i, err)
		}
	}

	f := newFlagsRefresher(db)
	f.tick(ctx)

	var snapshot struct {
		ID   string              `bson:"_id"`
		Data []flagSnapshotEntry `bson:"data"`
	}
	if err := db.Collection("status").FindOne(ctx, bson.M{"_id": "flags-snapshot"}).Decode(&snapshot); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if len(snapshot.Data) != 2 {
		t.Fatalf("snapshot has %d accounts, want 2 (the positive vote must be ignored): %+v",
			len(snapshot.Data), snapshot.Data)
	}

	top := snapshot.Data[0]
	if top.Author != "spammer" {
		t.Errorf("first account = %q, want spammer (4 downvotes)", top.Author)
	}
	if top.Count != 4 || top.VoterCount != 2 || top.PostCount != 2 {
		t.Errorf("spammer = count %d / voters %d / posts %d, want 4/2/2",
			top.Count, top.VoterCount, top.PostCount)
	}
	if len(top.Voters) != 2 || top.Voters[0].Voter != "alice" || top.Voters[0].Count != 2 {
		t.Errorf("spammer voters = %+v, want alice(2) first", top.Voters)
	}

	if second := snapshot.Data[1]; second.Author != "troll" || second.Count != 2 {
		t.Errorf("second account = %+v, want troll/2", second)
	}
}

// TestFlagSnapshotBSONShape pins the document shape the web reader decodes
// (steemdb-web GetFlags / flagSnapshotEntry): author, count, voter_count,
// post_count, voters[].voter/.count. Renaming a field here without the reader
// would silently blank the labs/flags page.
func TestFlagSnapshotBSONShape(t *testing.T) {
	entry := buildFlagSnapshotEntry(flagVoteRow{
		Author: "alice",
		Count:  4,
		Voters: []string{"bob", "bob", "carol", "dave"},
		Posts:  []string{"p1", "p2"},
	})

	raw, err := bson.Marshal(bson.M{"_id": "flags-snapshot", "data": bson.A{entry}})
	if err != nil {
		t.Fatalf("failed to marshal snapshot document: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("failed to decode snapshot document: %v", err)
	}

	if doc["_id"] != "flags-snapshot" {
		t.Errorf("_id = %v, want flags-snapshot", doc["_id"])
	}
	data, ok := doc["data"].(bson.A)
	if !ok || len(data) != 1 {
		t.Fatalf("data = %v, want one entry", doc["data"])
	}
	row, ok := data[0].(bson.M)
	if !ok {
		t.Fatalf("data[0] is %T, want bson.M", data[0])
	}

	for _, key := range []string{"author", "count", "voter_count", "post_count", "voters"} {
		if _, exists := row[key]; !exists {
			t.Errorf("snapshot row is missing %q (web decodes this exact key)", key)
		}
	}
	if row["author"] != "alice" || row["count"] != int32(4) {
		t.Errorf("author/count = %v/%v, want alice/4", row["author"], row["count"])
	}
	if row["voter_count"] != int32(3) {
		t.Errorf("voter_count = %v, want 3", row["voter_count"])
	}
	if row["post_count"] != int32(2) {
		t.Errorf("post_count = %v, want 2", row["post_count"])
	}

	voters, ok := row["voters"].(bson.A)
	if !ok || len(voters) != 3 {
		t.Fatalf("voters = %v, want 3 entries", row["voters"])
	}
	first, ok := voters[0].(bson.M)
	if !ok {
		t.Fatalf("voters[0] is %T, want bson.M", voters[0])
	}
	if first["voter"] != "bob" || first["count"] != int32(2) {
		t.Errorf("voters[0] = %v, want bob/2 (top flagger first)", first)
	}
}
