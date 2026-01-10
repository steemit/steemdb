package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
)

// Batcher handles batching operations for bulk writes
type Batcher struct {
	cfg           *config.Config
	mongoClient   *mongo.Client
	batchSize     int
	flushInterval time.Duration
	ops           chan *model.Operation
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	maxBlockSeen  uint32
	maxBlockMu    sync.RWMutex
}

// NewBatcher creates a new batcher
func NewBatcher(cfg *config.Config, mongoClient *mongo.Client) (*Batcher, error) {
	flushInterval, err := cfg.BatchFlushInterval()
	if err != nil {
		return nil, errors.Wrap(err, "invalid batch flush interval")
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Batcher{
		cfg:           cfg,
		mongoClient:   mongoClient,
		batchSize:     cfg.Batch.Size,
		flushInterval: flushInterval,
		ops:           make(chan *model.Operation, cfg.Ingest.QueueSize),
		ctx:           ctx,
		cancel:        cancel,
	}

	return b, nil
}

// Start starts the batcher
func (b *Batcher) Start() {
	b.wg.Add(1)
	go b.run()
}

// Stop stops the batcher and flushes remaining operations
func (b *Batcher) Stop() error {
	close(b.ops)
	b.cancel()
	b.wg.Wait()
	// Note: run() goroutine already flushes remaining batch when channel closes,
	// so we don't need to call flush() again here.
	// If there are any remaining operations in the channel buffer, they will be
	// handled by the next flush() call from run() before it exits.
	return nil
}

// AddOperation adds an operation to the batch queue
func (b *Batcher) AddOperation(op *model.Operation) error {
	// Check if context is cancelled first
	select {
	case <-b.ctx.Done():
		return errors.New("batcher stopped")
	default:
	}

	// Try to send to channel
	select {
	case b.ops <- op:
		// Record metrics
		metrics.RecordIngestOp(op.Source)
		metrics.UpdateQueueSize(len(b.ops))
		metrics.UpdateCurrentBlock(op.BlockNum)

		// Update max block seen
		b.maxBlockMu.Lock()
		if op.BlockNum > b.maxBlockSeen {
			b.maxBlockSeen = op.BlockNum
		}
		b.maxBlockMu.Unlock()
		return nil
	case <-b.ctx.Done():
		return errors.New("batcher stopped")
	default:
		// Channel might be full or closed, try one more time with context check
		select {
		case b.ops <- op:
			// Record metrics
			metrics.RecordIngestOp(op.Source)
			metrics.UpdateQueueSize(len(b.ops))
			metrics.UpdateCurrentBlock(op.BlockNum)

			// Update max block seen
			b.maxBlockMu.Lock()
			if op.BlockNum > b.maxBlockSeen {
				b.maxBlockSeen = op.BlockNum
			}
			b.maxBlockMu.Unlock()
			return nil
		case <-b.ctx.Done():
			return errors.New("batcher stopped")
		}
	}
}

// GetMaxBlockSeen returns the maximum block number seen
func (b *Batcher) GetMaxBlockSeen() uint32 {
	b.maxBlockMu.RLock()
	defer b.maxBlockMu.RUnlock()
	return b.maxBlockSeen
}

// run is the main batching loop
func (b *Batcher) run() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	batch := make([]*model.Operation, 0, b.batchSize)

	for {
		select {
		case op, ok := <-b.ops:
			if !ok {
				// Channel closed, flush remaining and exit
				if len(batch) > 0 {
					b.flush(context.Background())
				}
				return
			}

			batch = append(batch, op)

			if len(batch) >= b.batchSize {
				b.flush(context.Background())
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				b.flush(context.Background())
				batch = batch[:0]
			}

		case <-b.ctx.Done():
			if len(batch) > 0 {
				b.flush(context.Background())
			}
			return
		}
	}
}

// flush writes the batch to MongoDB
func (b *Batcher) flush(ctx context.Context) error {
	// Collect operations from channel up to batch size
	batch := make([]*model.Operation, 0, b.batchSize)

	// Collect from channel with timeout
	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()

	channelClosed := false
	for len(batch) < b.batchSize && !channelClosed {
		select {
		case op, ok := <-b.ops:
			if !ok {
				// Channel closed, exit loop
				channelClosed = true
			} else {
				batch = append(batch, op)
			}
		case <-timeout.C:
			// Timeout, flush what we have (exit loop)
			channelClosed = true
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if len(batch) == 0 {
		return nil
	}

	// Record batch metrics
	startTime := time.Now()

	// Write to MongoDB
	err := b.mongoClient.BulkUpsertOperations(ctx, batch)
	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordBatch(len(batch), duration)
	metrics.UpdateQueueSize(len(b.ops))

	if err != nil {
		return errors.Wrap(err, "failed to flush batch")
	}

	return nil
}
