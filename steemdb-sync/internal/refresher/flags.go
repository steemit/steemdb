package refresher

import (
	"context"
	"log"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// flagsSnapshotDepth is how many most-flagged authors the snapshot keeps
	// (the value the web endpoint's former aggregation ended with).
	flagsSnapshotDepth = 200
	// flagsTopVoters is how many downvoters are kept per flagged author.
	flagsTopVoters = 10
)

// flagsRefresher materialises the labs/flags leaderboard into
// status.flags-snapshot (read by web's GET /api/v1/labs/flags).
//
// Why a snapshot instead of a per-request aggregation: `vote` holds ~941M
// documents, ~7.7M of them with weight<0. The only index covering that slice
// is the partial {weight:1}, which carries no author/voter/permlink, so every
// request must FETCH all ~7.7M negative-vote documents. Measured on production
// the per-request $group form needs >100MB of memory (MongoDB code 292
// QueryExceededMemoryLimitNoDiskUseAllowed — the source of the endpoint's 500)
// and 145-293s once allowDiskUse is enabled. Running it here once per ticker
// and serving the result as a single document keeps the request path a point
// read.
//
// Gated by blockedForProcessor: `vote` is processor-derived, so a tick during
// cold start would snapshot a partial history.
type flagsRefresher struct {
	voteCol   *mongo.Collection
	statusCol *mongo.Collection
	gate      *catchupGate
}

func newFlagsRefresher(db *mongo.Database) *flagsRefresher {
	return &flagsRefresher{
		voteCol:   db.Collection("vote"),
		statusCol: db.Collection("status"),
		gate:      newCatchupGate(db),
	}
}

// flagVoteRow is one author's aggregation output. Voters holds one entry per
// downvote op (repeats expected: unvote/revote and multi-op flags) so the op
// count per voter can be folded here instead of in MongoDB.
type flagVoteRow struct {
	Author string   `bson:"_id"`
	Count  int      `bson:"count"`
	Voters []string `bson:"voters"`
	Posts  []string `bson:"posts"`
}

// flagVoterCount is one voter's downvote op count against one author.
type flagVoterCount struct {
	Voter string `bson:"voter"`
	Count int    `bson:"count"`
}

// flagSnapshotEntry is one snapshot row. The voter and permlink arrays are
// reduced to counts plus a top-N voter list: the arrays themselves carried
// ~40MB of names the SPA never read.
type flagSnapshotEntry struct {
	Author     string           `bson:"author"`
	Count      int              `bson:"count"`
	VoterCount int              `bson:"voter_count"`
	PostCount  int              `bson:"post_count"`
	Voters     []flagVoterCount `bson:"voters"`
}

func (f *flagsRefresher) tick(ctx context.Context) {
	if skip, reason := f.gate.blockedForProcessor(ctx); skip {
		log.Printf("[Refresher:Flags] Skipped: %s", reason)
		return
	}

	started := time.Now()

	// Single-stage $group by author: 215k groups holding one array per voter
	// and the distinct permlink set. Known to complete with allowDiskUse
	// (the multi-stage alternative has to spill ~7.7M intermediate groups,
	// which is slower).
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"weight": bson.M{"$lt": 0},
			},
		},
		{
			"$group": bson.M{
				"_id":    "$author",
				"count":  bson.M{"$sum": 1},
				"voters": bson.M{"$push": "$voter"},
				"posts":  bson.M{"$addToSet": "$permlink"},
			},
		},
		{
			"$sort": bson.M{"count": -1},
		},
		{
			"$limit": flagsSnapshotDepth,
		},
	}

	cursor, err := f.voteCol.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		log.Printf("[Refresher:Flags] Aggregation failed: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var rows []flagVoteRow
	if err := cursor.All(ctx, &rows); err != nil {
		log.Printf("[Refresher:Flags] Failed to decode aggregation: %v", err)
		return
	}

	data := make([]flagSnapshotEntry, 0, len(rows))
	for _, row := range rows {
		data = append(data, buildFlagSnapshotEntry(row))
	}

	if _, err := f.statusCol.UpdateOne(ctx,
		bson.M{"_id": "flags-snapshot"},
		bson.M{"$set": bson.M{"data": data, "updated_at": time.Now().UTC()}},
		options.Update().SetUpsert(true),
	); err != nil {
		log.Printf("[Refresher:Flags] Failed to write flags-snapshot: %v", err)
		return
	}

	log.Printf("[Refresher:Flags] Snapshot updated (%d accounts, took %s)",
		len(data), time.Since(started).Round(time.Millisecond))
}

// buildFlagSnapshotEntry folds one aggregation row into a snapshot entry:
// distinct voter and permlink counts, plus the top-N voters by op count
// (ties broken by name so the snapshot is deterministic).
func buildFlagSnapshotEntry(row flagVoteRow) flagSnapshotEntry {
	counts := make(map[string]int, len(row.Voters))
	for _, voter := range row.Voters {
		counts[voter]++
	}

	voters := make([]flagVoterCount, 0, len(counts))
	for voter, count := range counts {
		voters = append(voters, flagVoterCount{Voter: voter, Count: count})
	}
	sort.Slice(voters, func(i, j int) bool {
		if voters[i].Count != voters[j].Count {
			return voters[i].Count > voters[j].Count
		}
		return voters[i].Voter < voters[j].Voter
	})
	if len(voters) > flagsTopVoters {
		voters = voters[:flagsTopVoters]
	}

	return flagSnapshotEntry{
		Author:     row.Author,
		Count:      row.Count,
		VoterCount: len(counts),
		PostCount:  len(row.Posts),
		Voters:     voters,
	}
}
