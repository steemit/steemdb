package processor

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
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

	// Collections from the operations/blocks/meta sets
	opsCol    *drivermongo.Collection
	blocksCol *drivermongo.Collection
	metaCol   *drivermongo.Collection

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
		cfg:         ctx.Cfg,
		mongoClient: ctx.MongoClient,
		dispatcher:  dispatcher,
		cursor:      NewCursor(db),
		opsCol:      db.Collection("operations"),
		blocksCol:   db.Collection("blocks"),
		metaCol:     db.Collection("meta"),
		catchUpSleep: catchUpSleep,
	}, nil
}

// Run starts the main processing loop. It blocks until ctx is cancelled.
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

	log.Printf("[Processor] Starting from block %d", height+1)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Processor] Shutting down (last processed block: %d)", height)
			return ctx.Err()
		default:
		}

		nextBlock := height + 1

		// Fetch operations for nextBlock, ordered by _id to preserve op sequence.
		// _id format is "block:trx:op", so lexicographic order = chronological order within a block.
		ops, err := p.fetchOpsForBlock(ctx, nextBlock)
		if err != nil {
			log.Printf("[Processor] Error fetching ops for block %d: %v", nextBlock, err)
			time.Sleep(p.catchUpSleep)
			continue
		}

		// No ops for this block — either not ingested yet, or genuinely empty block.
		if len(ops) == 0 {
			// Check if the block itself exists. If it does, it's an empty block — advance.
			// If it doesn't, we haven't caught up with live_sync yet — wait.
			exists, err := p.blockExists(ctx, nextBlock)
			if err != nil {
				log.Printf("[Processor] Error checking block %d existence: %v", nextBlock, err)
				time.Sleep(p.catchUpSleep)
				continue
			}
			if !exists {
				// Block not yet ingested — wait for live_sync / cold_ingest to catch up
				time.Sleep(p.catchUpSleep)
				continue
			}
			// Block exists but has no ops — advance cursor without dispatching.
			// (The block itself was already written by live_sync/cold_ingest.)
			if err := p.cursor.Advance(ctx, nextBlock); err != nil {
				log.Printf("[Processor] Error advancing cursor for empty block %d: %v", nextBlock, err)
				continue
			}
			height = nextBlock
			continue
		}

		// Get block timestamp for the handlers.
		blockTS, err := p.getBlockTimestamp(ctx, nextBlock)
		if err != nil {
			log.Printf("[Processor] Error getting timestamp for block %d: %v", nextBlock, err)
			// Fall back to a zero time — handlers should handle this gracefully
			blockTS = time.Time{}
		}

		// Dispatch all ops in this block sequentially.
		errCount := p.dispatcher.DispatchBlock(ctx, ops, blockTS)

		// Advance cursor — this is the commit point.
		if err := p.cursor.Advance(ctx, nextBlock); err != nil {
			log.Printf("[Processor] Error advancing cursor to %d: %v", nextBlock, err)
			time.Sleep(p.catchUpSleep)
			continue
		}

		height = nextBlock

		// Periodic progress logging
		if nextBlock%10000 == 0 || errCount > 0 {
			log.Printf("[Processor] Processed block %d (%d ops, %d errors)", nextBlock, len(ops), errCount)
		}
	}
}

// fetchOpsForBlock retrieves all operations for a given block, ordered by _id.
func (p *Processor) fetchOpsForBlock(ctx context.Context, blockNum uint32) ([]*model.Operation, error) {
	filter := bson.M{"block_num": blockNum}
	opts := options.Find().SetSort(bson.M{"_id": 1})

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
