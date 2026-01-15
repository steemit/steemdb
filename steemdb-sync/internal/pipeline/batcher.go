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
	stopOnce      sync.Once
	maxBlockSeen  uint32
	maxBlockMu    sync.RWMutex
	// Track block information for each block number
	blocks   map[uint32]*blockInfo
	blocksMu sync.RWMutex
	// Track which blocks have been written to MongoDB
	blocksWritten   map[uint32]bool
	blocksWrittenMu sync.RWMutex
}

// blockInfo stores block information temporarily
type blockInfo struct {
	BlockNum  uint32
	BlockID   string
	Timestamp time.Time
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
		blocks:        make(map[uint32]*blockInfo),
		blocksWritten: make(map[uint32]bool),
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
	b.stopOnce.Do(func() {
		// Close the channel to let run() drain buffered operations and flush remaining batch.
		close(b.ops)
		// Cancel context to prevent new AddOperation calls from blocking/panicking on a closed channel.
		b.cancel()
	})
	b.wg.Wait()
	return nil
}

// AddBlockInfo stores block information for later writing
func (b *Batcher) AddBlockInfo(blockNum uint32, blockID string, timestamp time.Time) {
	b.blocksMu.Lock()
	defer b.blocksMu.Unlock()
	b.blocks[blockNum] = &blockInfo{
		BlockNum:  blockNum,
		BlockID:   blockID,
		Timestamp: timestamp,
	}
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
					_ = b.flushBatch(context.Background(), batch)
				}
				// Flush all remaining unwritten blocks before exit
				_ = b.flushUnwrittenBlocks(context.Background())
				return
			}

			batch = append(batch, op)

			if len(batch) >= b.batchSize {
				_ = b.flushBatch(context.Background(), batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				_ = b.flushBatch(context.Background(), batch)
				batch = batch[:0]
			}
			// Also flush unwritten blocks periodically (including block-only blocks)
			_ = b.flushUnwrittenBlocks(context.Background())
		}
	}
}

// flushBatch writes a provided batch to MongoDB.
// Note: Do NOT read from b.ops here; run() owns draining the channel.
func (b *Batcher) flushBatch(ctx context.Context, batch []*model.Operation) error {
	if len(batch) == 0 {
		return nil
	}

	// Record batch metrics
	startTime := time.Now()

	// Write operations to MongoDB
	err := b.mongoClient.BulkUpsertOperations(ctx, batch)
	if err != nil {
		return errors.Wrap(err, "failed to flush operations batch")
	}

	// Extract and write blocks for operations in this batch
	b.blocksMu.RLock()
	blocksToWrite := make([]*model.Block, 0)
	blockNums := make(map[uint32]bool)
	for _, op := range batch {
		if !blockNums[op.BlockNum] {
			blockNums[op.BlockNum] = true
			if blockInfo, ok := b.blocks[op.BlockNum]; ok {
				blocksToWrite = append(blocksToWrite, &model.Block{
					ID:               op.BlockNum,
					BlockNum:         op.BlockNum,
					BlockID:          blockInfo.BlockID,
					Timestamp:        blockInfo.Timestamp,
					TransactionCount: 0, // Will be updated by aggregation if needed
				})
			}
		}
	}
	b.blocksMu.RUnlock()

	// Write blocks to MongoDB
	if len(blocksToWrite) > 0 {
		if err := b.mongoClient.BulkUpsertBlocks(ctx, blocksToWrite); err != nil {
			return errors.Wrap(err, "failed to flush blocks batch")
		}

		// Mark blocks as written
		b.blocksWrittenMu.Lock()
		for _, block := range blocksToWrite {
			b.blocksWritten[block.BlockNum] = true
		}
		b.blocksWrittenMu.Unlock()
	}

	duration := time.Since(startTime)

	// Record metrics
	metrics.RecordBatch(len(batch), duration)
	metrics.UpdateQueueSize(len(b.ops))

	return nil
}

// FlushOperationsAndBlocks synchronously flushes operations and blocks to MongoDB
// This ensures data is written before returning (for ACK mechanism)
func (b *Batcher) FlushOperationsAndBlocks(ctx context.Context, ops []*model.Operation) error {
	if len(ops) == 0 {
		// Even if no operations, flush any unwritten blocks
		return b.flushUnwrittenBlocks(ctx)
	}

	// Write operations to MongoDB
	if err := b.mongoClient.BulkUpsertOperations(ctx, ops); err != nil {
		return errors.Wrap(err, "failed to flush operations")
	}

	// Extract and write blocks for these operations
	b.blocksMu.RLock()
	blocksToWrite := make([]*model.Block, 0)
	blockNums := make(map[uint32]bool)
	for _, op := range ops {
		if !blockNums[op.BlockNum] {
			blockNums[op.BlockNum] = true
			if blockInfo, ok := b.blocks[op.BlockNum]; ok {
				blocksToWrite = append(blocksToWrite, &model.Block{
					ID:               op.BlockNum,
					BlockNum:         op.BlockNum,
					BlockID:          blockInfo.BlockID,
					Timestamp:        blockInfo.Timestamp,
					TransactionCount: 0,
				})
			}
		}
	}
	b.blocksMu.RUnlock()

	// Write blocks to MongoDB
	if len(blocksToWrite) > 0 {
		if err := b.mongoClient.BulkUpsertBlocks(ctx, blocksToWrite); err != nil {
			return errors.Wrap(err, "failed to flush blocks")
		}

		// Mark blocks as written
		b.blocksWrittenMu.Lock()
		for _, block := range blocksToWrite {
			b.blocksWritten[block.BlockNum] = true
		}
		b.blocksWrittenMu.Unlock()
	}

	// Also flush any other unwritten blocks (block-only blocks)
	return b.flushUnwrittenBlocks(ctx)
}

// flushUnwrittenBlocks writes all blocks that haven't been written yet to MongoDB
// This includes block-only blocks (blocks without operations)
func (b *Batcher) flushUnwrittenBlocks(ctx context.Context) error {
	b.blocksMu.RLock()
	b.blocksWrittenMu.RLock()

	blocksToWrite := make([]*model.Block, 0)
	for blockNum, blockInfo := range b.blocks {
		if !b.blocksWritten[blockNum] {
			blocksToWrite = append(blocksToWrite, &model.Block{
				ID:               blockNum,
				BlockNum:         blockNum,
				BlockID:          blockInfo.BlockID,
				Timestamp:        blockInfo.Timestamp,
				TransactionCount: 0, // Will be updated by aggregation if needed
			})
		}
	}

	b.blocksWrittenMu.RUnlock()
	b.blocksMu.RUnlock()

	if len(blocksToWrite) == 0 {
		return nil
	}

	// Write blocks to MongoDB
	if err := b.mongoClient.BulkUpsertBlocks(ctx, blocksToWrite); err != nil {
		return errors.Wrap(err, "failed to flush unwritten blocks")
	}

	// Mark blocks as written
	b.blocksWrittenMu.Lock()
	for _, block := range blocksToWrite {
		b.blocksWritten[block.BlockNum] = true
	}
	b.blocksWrittenMu.Unlock()

	return nil
}
