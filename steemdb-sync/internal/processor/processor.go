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
	"github.com/steemit/steemdb-sync/internal/processor/handlers"
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
	cursor      cursorStore
	inserter    *handlers.MongoInserter

	// store abstracts the per-window MongoDB reads so the window loop's
	// cursor semantics can be unit tested without a live database.
	store windowStore

	// failures tracks consecutive per-op handler failures across replays of
	// the same window and implements the poison-op escape hatch (see
	// failedOpTracker). Nil-safe.
	failures *failedOpTracker

	catchUpSleep time.Duration
}

// cursorStore abstracts the processor cursor (read / advance) so window
// processing can be unit tested without MongoDB. *Cursor is the production
// implementation.
type cursorStore interface {
	Get(ctx context.Context) (uint32, error)
	Advance(ctx context.Context, blockNum uint32) error
}

// windowStore abstracts the per-window MongoDB reads so window processing
// can be unit tested without MongoDB. mongoWindowStore is the production
// implementation.
type windowStore interface {
	// WindowBlocks returns the metadata of existing blocks in [start, end],
	// ordered by block number.
	WindowBlocks(ctx context.Context, start, end uint32) ([]windowBlockMeta, error)
	// WindowOps returns all operations for blocks in [start, end], ordered
	// by (block_num, trx_index, op_index).
	WindowOps(ctx context.Context, start, end uint32) ([]*model.Operation, error)
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
		inserter:     ctx.Inserter,
		store:        newMongoWindowStore(db),
		failures:     newFailedOpTracker(ctx.Cfg.Processor.SkipErrorOpsAfterRetries),
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

// bufferLimit resolves the effective per-collection buffer cap from config.
func (p *Processor) bufferLimit() int {
	if p.cfg.Processor.BufferLimit > 0 {
		return p.cfg.Processor.BufferLimit
	}
	return defaultBufferLimit
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
	if p.inserter != nil {
		p.inserter.BeginBatch(p.bufferLimit())
		defer p.inserter.EndBatch()
	}
	log.Printf("[Processor] Starting from block %d (window=%d)", height+1, windowSize)

	var lastLogged uint32 = height

	// Window-phase timing breakdown, logged every 50 windows to locate
	// where wall time goes when throughput regresses.
	var (
		nWin                int
		fetchMs, dispatchMs time.Duration
		flushMs, cursorMs   time.Duration
	)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Processor] Shutting down (last processed block: %d)", height)
			return ctx.Err()
		default:
		}

		prevHeight := height
		oc := p.processWindow(ctx, height)
		if !oc.committed {
			// The window waited (nothing ingested yet, or a missing block
			// header — see processWindow) or failed (query, flush, cursor
			// error). The cursor is untouched: retry the same window.
			time.Sleep(p.catchUpSleep)
			continue
		}
		height = oc.newHeight

		fetchMs += oc.fetchMs
		dispatchMs += oc.dispatchMs
		flushMs += oc.flushMs
		cursorMs += oc.cursorMs

		nWin++
		if nWin%50 == 0 {
			log.Printf("[Processor] Window breakdown (50w avg): fetch=%dms dispatch=%dms flush=%dms cursor=%dms",
				fetchMs.Milliseconds()/int64(50), dispatchMs.Milliseconds()/int64(50),
				flushMs.Milliseconds()/int64(50), cursorMs.Milliseconds()/int64(50))
			fetchMs, dispatchMs, flushMs, cursorMs = 0, 0, 0, 0
		}

		// Periodic progress logging (every ~10k blocks)
		if height-lastLogged >= 10000 {
			log.Printf("[Processor] Processed through block %d (window=%d blocks, %d ops, %d errors)",
				height, height-prevHeight, oc.opsCount, oc.errCount)
			lastLogged = height
		}
		if oc.skippedCount > 0 {
			// Companion to the per-op SKIP log in processWindow: a window
			// summary so the sacrifice is visible at window granularity too.
			log.Printf("[Processor] Window %d-%d committed while skipping %d poison op(s); their derived writes were NOT created",
				prevHeight+1, height, oc.skippedCount)
		}
	}
}

// windowOutcome reports what one window attempt did. newHeight always
// carries the height the cursor should be read at next (the input height
// unless the window committed); the counters describe the attempt itself —
// meaningful for logging even when the window was held.
type windowOutcome struct {
	// committed is true when the window was fully dispatched, flushed, and
	// the cursor advanced — the window's commit point. When false the cursor
	// was left untouched and the same window must be retried; any handler
	// writes already buffered are covered by the replay-idempotency
	// invariant (docs/rules/processor-write-ordering.md).
	committed bool
	// newHeight is the new cursor position (== input height unless committed).
	newHeight uint32
	opsCount  int
	// errCount is the number of ops whose handler failed in this attempt.
	// A committed window always has errCount == 0 — the error gate below
	// holds any window with handler errors.
	errCount int
	// skippedCount is the number of poison ops deliberately not dispatched
	// (they exhausted processor.skip_error_ops_after_retries). Their derived
	// writes were NOT created.
	skippedCount int

	// Phase timings for the periodic window-breakdown log.
	fetchMs, dispatchMs, flushMs, cursorMs time.Duration
}

// processWindow attempts one window [height+1, height+windowSize].
//
// Ordering contract (what makes cursor semantics crash-safe):
//  1. plan the effective range from the block headers actually present —
//     a gap at the window HEAD holds the window (no dispatch, no cursor
//     advance) instead of skipping over the block;
//  2. refuse to dispatch ops of a block whose header is missing (a zero
//     timestamp would pollute _ts downstream) — hold and wait;
//  3. flush every buffered write (even when the window is about to be held —
//     idempotent upserts make the partial landing safe) and refuse to
//     advance when any op's handler errored or any flush failed: replay
//     never returns for a block the cursor has passed, so committing past
//     an error would permanently drop that op's derived writes (F5);
//  4. only then advance the cursor. Every wait/error path above leaves the
//     cursor untouched, so the next iteration replays the same window.
func (p *Processor) processWindow(ctx context.Context, height uint32) windowOutcome {
	oc := windowOutcome{newHeight: height}

	start := height + 1
	end := start + uint32(p.windowSize()) - 1

	windowStart := time.Now()
	tFetch := time.Now()

	// Window block metadata: existence and timestamps in one query.
	windowBlocks, err := p.store.WindowBlocks(ctx, start, end)
	if err != nil {
		log.Printf("[Processor] Error fetching window blocks %d-%d: %v", start, end, err)
		return oc
	}
	if len(windowBlocks) == 0 {
		// Nothing ingested in the window yet — wait for live_sync / cold_ingest.
		return oc
	}

	// Effective window end: the last contiguously existing block. A gap
	// inside the window means ingest has not reached this range yet.
	effectiveEnd := planWindow(start, windowBlocks)
	if effectiveEnd < start {
		// Window head gap: the block header at `start` is missing while
		// later blocks of the window exist (partial repair failure, or a
		// raced header write after an ops-first ingest). Advancing here
		// would permanently skip the head block's ops with no alarm — hold
		// the cursor and wait for the header to land via ingest retry or
		// repair (sync review finding F1).
		log.Printf("[Processor] Window %d-%d head gap: block header %d missing while %d later block(s) exist; holding cursor, waiting for ingest/repair",
			start, end, start, len(windowBlocks))
		return oc
	}

	windowTS := make(map[uint32]time.Time, len(windowBlocks))
	for _, m := range windowBlocks {
		windowTS[m.BlockNum] = m.Timestamp
	}

	// All operations of the window in one query, ordered by (block, trx, op).
	ops, err := p.store.WindowOps(ctx, start, effectiveEnd)
	if err != nil {
		log.Printf("[Processor] Error fetching ops for window %d-%d: %v", start, effectiveEnd, err)
		return oc
	}

	// Ops must never be dispatched for a block without a header: handlers
	// would stamp every derived document of that block with a zero _ts, and
	// same-filter upserts (witness_vote's _ts filter) would silently drop
	// documents. Hold the window until the header lands (sync review
	// finding F1). After planWindow this is unreachable for consistent
	// data — the check enforces the invariant instead of relying on it.
	if blockNum, missing := firstOpsBlockWithoutHeader(ops, windowTS); missing {
		log.Printf("[Processor] Window %d-%d: ops exist for block %d but its block header is missing; holding cursor, waiting for header",
			start, effectiveEnd, blockNum)
		return oc
	}

	tDispatch := time.Now()
	oc.fetchMs = tDispatch.Sub(tFetch)

	// Dispatch grouped by block to preserve the per-block error accounting
	// and intra-block ordering semantics of DispatchBlock. Ops that have
	// exhausted their retry budget (poison ops) are skipped here — the only
	// path that deliberately forgoes an op's derived writes (loud log below).
	var failed []string
	var dispatchedIDs []string
	skipped := 0
	var (
		currentBlock uint32
		blockOps     []*model.Operation
		blockTS      time.Time
	)
	dispatchGroup := func() {
		if len(blockOps) > 0 {
			failed = append(failed, p.dispatcher.DispatchBlock(ctx, blockOps, blockTS)...)
			blockOps = nil
		}
	}
	for _, op := range ops {
		if op.BlockNum != currentBlock {
			dispatchGroup()
			currentBlock = op.BlockNum
			// Presence guaranteed by firstOpsBlockWithoutHeader above; a
			// missing entry no longer degrades to a zero timestamp.
			blockTS = windowTS[op.BlockNum]
		}
		if skip, streak, threshold := p.failures.skipInfo(op.ID); skip {
			log.Printf("[Processor] SKIP op %s (type=%s, block=%d): handler failed %d consecutive window attempts (processor.skip_error_ops_after_retries=%d); advancing past it — derived writes for this op are NOT created. Investigate the op and re-derive manually if needed.",
				op.ID, op.OpType, op.BlockNum, streak, threshold)
			skipped++
			continue
		}
		dispatchedIDs = append(dispatchedIDs, op.ID)
		blockOps = append(blockOps, op)
	}
	dispatchGroup()

	// Bookkeeping for the poison-op tracker must happen before any early
	// return below so a held window's retry accounting stays correct even
	// when FlushAll also fails.
	p.failures.recordAttempt(dispatchedIDs, failed)

	tFlush := time.Now()
	oc.dispatchMs = tFlush.Sub(tDispatch)
	oc.opsCount = len(ops)

	// Flush all buffered writes even when handler errors will hold the window
	// below: landing the successfully-dispatched writes now is safe — buffered
	// writes are idempotent upserts and direct-write handlers carry their own
	// replay guards (last_applied_op) — and it leaves the replay to converge
	// on the failing ops instead of repeating the whole buffer. On flush
	// error the buffers keep their contents (see MongoInserter.flushBucketLocked)
	// and the window replays.
	var flushErr error
	if p.inserter != nil {
		flushErr = p.inserter.FlushAll(ctx)
	}

	tCursor := time.Now()
	oc.flushMs = tCursor.Sub(tFlush)

	if flushErr != nil {
		log.Printf("[Processor] Error flushing window %d-%d buffers: %v", start, effectiveEnd, flushErr)
		if len(failed) == 0 {
			return oc
		}
		// Handler errors are pending too — fall through to the gate below,
		// which logs the more specific reason for holding the window.
	}
	if len(failed) > 0 {
		// Handler-error gate (sync review finding F5): the cursor must not
		// advance past an op whose handler failed — replay never returns for
		// it, so committing here would permanently drop its derived writes.
		// Hold the window; the next iteration replays it (idempotent writes).
		log.Printf("[Processor] Window %d-%d held: %d/%d op(s) failed their handler; cursor not advanced — window replays",
			start, effectiveEnd, len(failed), len(ops))
		oc.errCount = len(failed)
		oc.skippedCount = skipped
		return oc
	}

	// Advance the cursor — the window's commit point. A crash before this
	// replays the whole window; handler idempotency makes that safe.
	if err := p.cursor.Advance(ctx, effectiveEnd); err != nil {
		log.Printf("[Processor] Error advancing cursor to %d: %v", effectiveEnd, err)
		return oc
	}
	p.failures.reset()

	oc.cursorMs = time.Since(tCursor)
	oc.committed = true
	oc.newHeight = effectiveEnd
	oc.errCount = len(failed)
	oc.skippedCount = skipped

	metrics.RecordWindow(int(effectiveEnd-start+1), len(ops), time.Since(windowStart))

	return oc
}

// planWindow computes the effective window end: the last block number,
// counting from `start`, whose block header exists contiguously (input is
// sorted by block number). It returns start-1 when the head block's header
// is missing — including the empty-window case — which the caller must
// treat as "process nothing, advance nothing, retry later": skipping ahead
// would leave the head block's ops permanently underived.
//
// start is always >= 1 (it is computed as cursor height + 1), so the
// start-1 sentinel cannot underflow.
func planWindow(start uint32, windowBlocks []windowBlockMeta) uint32 {
	effectiveEnd := start - 1
	expect := start
	for _, m := range windowBlocks {
		if m.BlockNum != expect {
			break
		}
		effectiveEnd = m.BlockNum
		expect++
	}
	return effectiveEnd
}

// firstOpsBlockWithoutHeader returns the first block (ops are sorted by
// block_num) that carries operations but has no entry in the window's block
// header timestamps. Such ops must not be dispatched: handlers would receive
// a zero time.Time and stamp it into every derived document of the block.
func firstOpsBlockWithoutHeader(ops []*model.Operation, windowTS map[uint32]time.Time) (uint32, bool) {
	for _, op := range ops {
		if _, ok := windowTS[op.BlockNum]; !ok {
			return op.BlockNum, true
		}
	}
	return 0, false
}

// windowBlockMeta is the existence/timestamp record for one block in a window.
type windowBlockMeta struct {
	BlockNum  uint32    `bson:"_id"`
	Timestamp time.Time `bson:"timestamp"`
}

// mongoWindowStore reads window block metadata and operations from MongoDB.
type mongoWindowStore struct {
	opsCol    *drivermongo.Collection
	blocksCol *drivermongo.Collection
}

// newMongoWindowStore builds the production windowStore over the
// operations/blocks collections of db.
func newMongoWindowStore(db *drivermongo.Database) *mongoWindowStore {
	return &mongoWindowStore{
		opsCol:    db.Collection("operations"),
		blocksCol: db.Collection("blocks"),
	}
}

// WindowBlocks returns the metadata of existing blocks in [start, end],
// ordered by block number.
func (s *mongoWindowStore) WindowBlocks(ctx context.Context, start, end uint32) ([]windowBlockMeta, error) {
	cursor, err := s.blocksCol.Find(ctx,
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

// WindowOps retrieves all operations for blocks in [start, end], ordered by
// block, transaction, and op index.
// Sort by the numeric trx_index and op_index fields (NOT by _id string), because the
// _id format "{block}:{trx}:{op}" sorts lexicographically and would misorder indexes ≥ 10
// (e.g. "100:10:0" sorts before "100:2:0" as strings).
func (s *mongoWindowStore) WindowOps(ctx context.Context, start, end uint32) ([]*model.Operation, error) {
	opts := options.Find().SetSort(bson.D{
		{Key: "block_num", Value: 1},
		{Key: "trx_index", Value: 1},
		{Key: "op_index", Value: 1},
	})
	cursor, err := s.opsCol.Find(ctx, bson.M{
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

// GetHeight returns the last processed block height (for metrics / monitoring).
func (p *Processor) GetHeight(ctx context.Context) (uint32, error) {
	return p.cursor.Get(ctx)
}
