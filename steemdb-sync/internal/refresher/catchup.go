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
// Two data frontiers exist, so there are two gate variants:
//
//   - blocked(): the frontier is the ingest frontier (highest block in the
//     blocks collection). Use for aggregations over ingest-written data
//     (stats counts the operations collection) and for chain-head RPC
//     snapshots (account_rescan), which only care how far the local
//     database is behind the committed head.
//   - blockedForProcessor(): the frontier is the processor cursor
//     (status.processor_height). Use for aggregations over
//     processor-derived collections (clients aggregates comment): those
//     collections only advance with the processor, which lags the ingest
//     frontier during cold start — ingest can be fully caught up while the
//     processor is still days behind, and a tick then computes a
//     partial-window figure.
//
// Rationale (the "data grooming waits for catch-up" rule): over partially
// ingested data the tickers' output is a partial-window figure —
// overwrite-based snapshots (status counters) would self-heal on the next
// post-catch-up tick, but append-only records (clients_history day rows)
// would keep the under-counted day forever.
//
// Head semantics mirror processor.AccountRefresher.isCatchingUp:
//   - head = live_sync's committed meta.max_block,
//   - a missing or stale meta (head < frontier) falls back to the frontier's
//     own ceiling — for the processor variant that is the blocks collection
//     max, exactly like isCatchingUp.
//
// fail-safe: any read error means skip.
type catchupGate struct {
	metaCol   *mongo.Collection
	blocksCol *mongo.Collection
	statusCol *mongo.Collection
}

func newCatchupGate(db *mongo.Database) *catchupGate {
	return &catchupGate{
		metaCol:   db.Collection("meta"),
		blocksCol: db.Collection("blocks"),
		statusCol: db.Collection("status"),
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

// blockedForProcessor reports whether a tick over processor-derived data
// (e.g. the comment collection that clients aggregates) must skip. The
// frontier is the processor cursor, not the ingest frontier: during cold
// start the processor lags behind ingest, so blocked() would open the gate
// while the derived data is still partial.
//
// Mirrors processor.AccountRefresher.isCatchingUp: head = meta.max_block,
// falling back to the blocks collection max when meta is missing, unreadable,
// or stale (head < cursor).
func (g *catchupGate) blockedForProcessor(ctx context.Context) (bool, string) {
	frontier, err := g.processorHeight(ctx)
	if err != nil {
		return true, "failed to read processor cursor"
	}
	if frontier == 0 {
		return true, "processor cursor not established"
	}

	head, exists, err := g.syncedMaxBlock(ctx)
	if err != nil || !exists || head < frontier {
		// Same fallback as processor.AccountRefresher.isCatchingUp: trust
		// the blocks collection when meta is missing or stale.
		var fbErr error
		head, fbErr = g.latestBlock(ctx)
		if fbErr != nil {
			return true, "failed to read local data frontier"
		}
	}

	if gap := head - frontier; gap > coldStartThreshold {
		return true, fmt.Sprintf("processor catching up (processor %d, head %d, gap %d > %d)",
			frontier, head, gap, coldStartThreshold)
	}
	return false, ""
}

// processorHeight returns the processor cursor (0 when not initialized).
func (g *catchupGate) processorHeight(ctx context.Context) (int64, error) {
	var doc struct {
		Value int64 `bson:"value"`
	}
	err := g.statusCol.FindOne(ctx, bson.M{"_id": "processor_height"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, nil
		}
		return 0, err
	}
	return doc.Value, nil
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
