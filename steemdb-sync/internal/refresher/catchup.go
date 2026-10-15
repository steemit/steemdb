package refresher

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// coldStartThreshold is the number of blocks the local data may be behind the
// chain head before aggregation tickers skip their tick. Same value as the
// processor's AccountRefresher/CommentRescanner gate: within ~50 minutes of
// chain time counts as "caught up".
const coldStartThreshold int64 = 1000

// catchupGate reports whether local-data aggregation tickers must skip.
//
// Rationale (the "data grooming waits for catch-up" rule): stats and clients
// aggregate the ingested data. Over a partially ingested database their output
// is a partial-window figure — overwrite-based snapshots (status counters)
// would self-heal on the next post-catch-up tick, but append-only records
// (clients_history day rows) would keep the under-counted day forever.
//
// Frontier/head semantics mirror processor.AccountRefresher.isCatchingUp:
//   - frontier = highest block in the blocks collection (what the local data
//     actually contains),
//   - head     = live_sync's committed meta.max_block.
//
// fail-safe: any read error means skip.
type catchupGate struct {
	metaCol   *mongo.Collection
	blocksCol *mongo.Collection
}

func newCatchupGate(db *mongo.Database) *catchupGate {
	return &catchupGate{
		metaCol:   db.Collection("meta"),
		blocksCol: db.Collection("blocks"),
	}
}

// blocked reports whether the tick must be skipped, with the reason for logging.
func (g *catchupGate) blocked(ctx context.Context) (bool, string) {
	local, err := g.latestBlock(ctx)
	if err != nil {
		return true, "failed to read local data frontier"
	}
	if local == 0 {
		return true, "no blocks ingested yet"
	}

	head, exists, err := g.syncedMaxBlock(ctx)
	if err != nil {
		return true, "failed to read meta.sync_state"
	}
	if !exists {
		// No committed sync cursor: live_sync has never committed, so
		// everything local is mid-catch-up data.
		return true, "meta.sync_state missing (live_sync never committed)"
	}
	if head < local {
		// Same stale-meta fallback as processor.AccountRefresher.isCatchingUp:
		// trust the blocks collection over a stale cursor.
		return false, ""
	}

	if gap := head - local; gap > coldStartThreshold {
		return true, fmt.Sprintf("catching up (frontier %d, head %d, gap %d > %d)",
			local, head, gap, coldStartThreshold)
	}
	return false, ""
}

// latestBlock returns the highest ingested block number (0 when empty).
func (g *catchupGate) latestBlock(ctx context.Context) (int64, error) {
	var doc struct {
		ID int64 `bson:"_id"`
	}
	err := g.blocksCol.FindOne(ctx, bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}),
	).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, err
	}
	return doc.ID, nil
}

// syncedMaxBlock returns live_sync's committed max_block and whether the doc exists.
func (g *catchupGate) syncedMaxBlock(ctx context.Context) (int64, bool, error) {
	var doc struct {
		MaxBlock int64 `bson:"max_block"`
	}
	err := g.metaCol.FindOne(ctx, bson.M{"_id": "sync_state"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, false, nil
		}
		return 0, false, err
	}
	return doc.MaxBlock, true, nil
}
