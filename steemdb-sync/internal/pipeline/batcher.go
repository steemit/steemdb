package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
)

// Batcher handles batching operations for bulk writes
type Batcher struct {
	cfg          *config.Config
	mongoClient  *mongo.Client
	batchSize    int
	flushInterval time.Duration
	ops          chan *model.Operation
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	maxBlockSeen uint32
	maxBlockMu   sync.RWMutex
}

// NewBatcher creates a new batcher
func NewBatcher(cfg *config.Config, mongoClient *mongo.Client) (*Batcher, error) {
	flushInterval, err := cfg.BatchFlushInterval()
	if err != nil {
		return nil, errors.Wrap(err, "invalid batch flush interval")
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Batcher{
		cfg:          cfg,
		mongoClient:  mongoClient,
		batchSize:    cfg.Batch.Size,
		flushInterval: flushInterval,
		ops:          make(chan *model.Operation, cfg.Ingest.QueueSize),
		ctx:          ctx,
		cancel:       cancel,
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
	return b.flush(context.Background())
}

// AddOperation adds an operation to the batch queue
func (b *Batcher) AddOperation(op *model.Operation) error {
	select {
	case b.ops <- op:
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

	for len(batch) < b.batchSize {
		select {
		case op, ok := <-b.ops:
			if !ok {
				// Channel closed
				break
			}
			batch = append(batch, op)
		case <-timeout.C:
			// Timeout, flush what we have
			break
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if len(batch) == 0 {
		return nil
	}

	// Write to MongoDB
	if err := b.mongoClient.BulkUpsertOperations(ctx, batch); err != nil {
		return errors.Wrap(err, "failed to flush batch")
	}

	return nil
}
