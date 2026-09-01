package processor

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Processor is the main sequential consumer of the operations collection.
// It reads blocks of operations in order, dispatches them to handlers,
// and advances a cursor in the status collection.
type Processor struct {
	cfg         *config.Config
	mongoClient *mongo.Client
	dispatcher  *Dispatcher
	cursor      *Cursor

	// Collections from the operations/blocks sets
	opsCol    *drivermongo.Collection
	blocksCol *drivermongo.Collection

	catchUpSleep time.Duration
}

// NewProcessor creates a new Processor.
func NewProcessor(ctx *Context, dispatcher *Dispatcher) (*Processor, error) {
	catchUpSleep, err := ctx.Cfg.ProcessorCatchUpSleep()
	if err != nil {
		return nil, errors.Wrap(err, "invalid processor.catch_up_sleep")
	}

	db := ctx.MongoClient.Database()

	return &Processor{
		cfg:          ctx.Cfg,
		mongoClient:  ctx.MongoClient,
		dispatcher:   dispatcher,
		cursor:       NewCursor(db),
		opsCol:       db.Collection("operations"),
		blocksCol:    db.Collection("blocks"),
		catchUpSleep: catchUpSleep,
	}, nil
}

// Default window tuning (see docs/rules/processor-write-ordering.md and the
// batching design): window = number of blocks fetched/dispatched per loop
// iteration; buffer limit is enforced by the inserter (P3).
const (
	defaultWindowSize  = 64
	defaultBufferLimit = 5000
)

// windowSize resolves the effective window size from config.
func (p *Processor) windowSize() int {
	if p.cfg.Processor.WindowSize > 0 {
		return p.cfg.Processor.WindowSize
	}
	return defaultWindowSize
}

// Run starts the main processing loop. It blocks until ctx is cancelled.
//
// Blocks are processed in windows: one query fetches the window's block
// metadata (existence + timestamps), one query fetches all its operations,
// handlers run op by op in (block, trx, op) order, and the cursor advances
// once per window after all handler writes complete. A crash before the
// cursor advance replays the whole window — handler writes must therefore be
// idempotent (see docs/rules/processor-write-ordering.md).
func (p *Processor) Run(ctx context.Context) error {
	// Determine starting height
	height, err := p.cursor.Get(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read cursor")
	}

	// Allow override via config (start_height > 0 means force restart from there)
	if p.cfg.Processor.StartHeight > 0 {
		log.Printf("[Processor] Override start height: %d (cursor was %d)", p.cfg.Processor.StartHeight, height)
		height = p.cfg.Processor.StartHeight - 1 // cursor = last processed, so -1
	}

	windowSize := p.windowSize()
	log.Printf("[Processor] Starting from block %d (window=%d)", height+1, windowSize)

	var lastLogged uint32 = height

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Processor] Shutting down (last processed block: %d)", height)
			return ctx.Err()
		default:
		}

		start := height + 1
		end := start + uint32(windowSize) - 1

		// Window block metadata: existence and timestamps in one query.
		windowBlocks, err := p.fetchWindowBlocks(ctx, start, end)
		if err != nil {
			log.Printf("[Processor] Error fetching window blocks %d-%d: %v", start, end, err)
			time.Sleep(p.catchUpSleep)
			continue
		}
		if len(windowBlocks) == 0 {
			// Nothing ingested in the window yet — wait for live_sync / cold_ingest.
			time.Sleep(p.catchUpSleep)
			continue
		}

		// Effective window end: the last contiguously existing block. A gap
		// inside the window means ingest has not reached this range yet.
		effectiveEnd := start
		expect := start
		for _, m := range windowBlocks {
			if m.BlockNum != expect {
				break
			}
			effectiveEnd = m.BlockNum
			expect++
		}
		windowTS := make(map[uint32]time.Time, len(windowBlocks))
		for _, m := range windowBlocks {
			windowTS[m.BlockNum] = m.Timestamp
		}

		windowStart := time.Now()

		// All operations of the window in one query, ordered by (block, trx, op).
		ops, err := p.fetchOpsForWindow(ctx, start, effectiveEnd)
		if err != nil {
			log.Printf("[Processor] Error fetching ops for window %d-%d: %v", start, effectiveEnd, err)
			time.Sleep(p.catchUpSleep)
			continue
		}

		// Dispatch grouped by block to preserve the per-block error accounting
		// and intra-block ordering semantics of DispatchBlock.
		errCount := 0
		var (
			currentBlock uint32
			blockOps     []*model.Operation
			blockTS      time.Time
		)
		dispatchGroup := func() {
			if len(blockOps) > 0 {
				errCount += p.dispatcher.DispatchBlock(ctx, blockOps, blockTS)
			}
		}
		for _, op := range ops {
			if op.BlockNum != currentBlock {
				dispatchGroup()
				currentBlock = op.BlockNum
				blockOps = nil
				blockTS = windowTS[op.BlockNum] // zero time if missing — handlers tolerate it
			}
			blockOps = append(blockOps, op)
		}
		dispatchGroup()

		// Advance the cursor — the window's commit point. A crash before this
		// replays the whole window; handler idempotency makes that safe.
		if err := p.cursor.Advance(ctx, effectiveEnd); err != nil {
			log.Printf("[Processor] Error advancing cursor to %d: %v", effectiveEnd, err)
			time.Sleep(p.catchUpSleep)
			continue
		}
		height = effectiveEnd

		metrics.RecordWindow(int(effectiveEnd-start+1), len(ops), time.Since(windowStart))

		// Periodic progress logging (every ~10k blocks)
		if height-lastLogged >= 10000 {
			log.Printf("[Processor] Processed through block %d (window=%d blocks, %d ops, %d errors)",
				height, effectiveEnd-start+1, len(ops), errCount)
			lastLogged = height
		}
		if errCount > 0 {
			log.Printf("[Processor] Window %d-%d completed with %d handler errors", start, effectiveEnd, errCount)
		}
	}
}

// windowBlockMeta is the existence/timestamp record for one block in a window.
type windowBlockMeta struct {
	BlockNum  uint32    `bson:"_id"`
	Timestamp time.Time `bson:"timestamp"`
}

// fetchWindowBlocks returns the metadata of existing blocks in [start, end],
// ordered by block number.
func (p *Processor) fetchWindowBlocks(ctx context.Context, start, end uint32) ([]windowBlockMeta, error) {
	cursor, err := p.blocksCol.Find(ctx,
		bson.M{"_id": bson.M{"$gte": start, "$lte": end}},
		options.Find().
			SetProjection(bson.M{"_id": 1, "timestamp": 1}).
			SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query window blocks %d-%d", start, end)
	}
	defer cursor.Close(ctx)

	metas := make([]windowBlockMeta, 0, end-start+1)
	for cursor.Next(ctx) {
		var m windowBlockMeta
		if err := cursor.Decode(&m); err != nil {
			return nil, errors.Wrap(err, "failed to decode window block meta")
		}
		metas = append(metas, m)
	}
	return metas, cursor.Err()
}

// fetchOpsForWindow retrieves all operations for blocks in [start, end],
// ordered by block, transaction, and op index.
// Sort by the numeric trx_index and op_index fields (NOT by _id string), because the
// _id format "{block}:{trx}:{op}" sorts lexicographically and would misorder indexes ≥ 10
// (e.g. "100:10:0" sorts before "100:2:0" as strings).
func (p *Processor) fetchOpsForWindow(ctx context.Context, start, end uint32) ([]*model.Operation, error) {
	opts := options.Find().SetSort(bson.D{
		{Key: "block_num", Value: 1},
		{Key: "trx_index", Value: 1},
		{Key: "op_index", Value: 1},
	})
	cursor, err := p.opsCol.Find(ctx, bson.M{
		"block_num": bson.M{"$gte": start, "$lte": end},
	}, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch ops for window %d-%d", start, end)
	}
	defer cursor.Close(ctx)

	var ops []*model.Operation
	if err := cursor.All(ctx, &ops); err != nil {
		return nil, errors.Wrap(err, "failed to decode window ops")
	}
	return ops, nil
}

// fetchOpsForBlock retrieves all operations for a given block, ordered by tx/op index.
// Sort by the numeric trx_index and op_index fields (NOT by _id string), because the
// _id format "{block}:{trx}:{op}" sorts lexicographically and would misorder indexes ≥ 10
// (e.g. "100:10:0" sorts before "100:2:0" as strings).
func (p *Processor) fetchOpsForBlock(ctx context.Context, blockNum uint32) ([]*model.Operation, error) {
	filter := bson.M{"block_num": blockNum}
	opts := options.Find().SetSort(bson.D{
		{Key: "trx_index", Value: 1},
		{Key: "op_index", Value: 1},
	})

	cursor, err := p.opsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find ops for block %d", blockNum)
	}
	defer cursor.Close(ctx)

	var ops []*model.Operation
	if err := cursor.All(ctx, &ops); err != nil {
		return nil, errors.Wrapf(err, "failed to decode ops for block %d", blockNum)
	}
	return ops, nil
}

// blockExists checks if a block document exists in the blocks collection.
func (p *Processor) blockExists(ctx context.Context, blockNum uint32) (bool, error) {
	count, err := p.blocksCol.CountDocuments(ctx, bson.M{"_id": blockNum})
	if err != nil {
		return false, errors.Wrapf(err, "failed to check block %d", blockNum)
	}
	return count > 0, nil
}

// getBlockTimestamp retrieves the timestamp of a block.
func (p *Processor) getBlockTimestamp(ctx context.Context, blockNum uint32) (time.Time, error) {
	var doc struct {
		Timestamp time.Time `bson:"timestamp"`
	}
	err := p.blocksCol.FindOne(ctx, bson.M{"_id": blockNum}).Decode(&doc)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to get timestamp for block %d", blockNum)
	}
	return doc.Timestamp, nil
}

// GetHeight returns the last processed block height (for metrics / monitoring).
func (p *Processor) GetHeight(ctx context.Context) (uint32, error) {
	return p.cursor.Get(ctx)
}
