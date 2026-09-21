package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdkv2 "github.com/steemit/steemgosdk/api/v2"
	"github.com/steemit/steemutil/protocol"
	"go.mongodb.org/mongo-driver/bson"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// Tuning defaults; overridable via config (live_sync.*) or env.
// Concurrency and chunk defaults come from the SDK integration baseline
// (93 blocks/s at concurrency 32 for get_block; ops double the call count).
const (
	defaultChunkSize       = 256
	defaultFollowThreshold = 100
	defaultConcurrency     = 32
	followPollInterval     = 2 * time.Second
	headRefreshInterval    = 15 * time.Second
	retrySleep             = 3 * time.Second
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	chunkSize := cfg.LiveSync.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	followThreshold := cfg.LiveSync.FollowThreshold
	if followThreshold <= 0 {
		followThreshold = defaultFollowThreshold
	}
	concurrency := cfg.LiveSync.RPCConcurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	mongoClient, err := mongo.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB client: %v", err)
	}
	defer mongoClient.Close(context.Background())

	api := sdkv2.NewAPI(cfg.RPC.Endpoint, sdkv2.WithConcurrency(concurrency))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-sigChan
		log.Printf("[LiveSync] Received %v, shutting down (committed height is meta.max_block; uncommitted chunks replay on restart)", s)
		cancel()
	}()

	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		server := &http.Server{Addr: ":9091", Handler: metricsMux}
		log.Printf("[LiveSync] Starting metrics server on :9091/metrics")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[LiveSync] Metrics server error: %v", err)
		}
	}()

	if err := run(ctx, api, mongoClient, chunkSize, followThreshold); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("[LiveSync] Fatal: %v", err)
	}
}

// run is the main loop: batch catch-up while the gap exceeds the follow
// threshold, single-block following near the chain head. The commit point of
// a batch chunk (and of a followed block) is meta.max_block; all writes are
// idempotent upserts, so a crash simply replays the uncommitted chunk.
func run(ctx context.Context, api *sdkv2.API, mongoClient *mongo.Client, chunkSize, followThreshold int) error {
	// Resume point: the higher of meta.max_block and the blocks collection's
	// highest block. A replay populates the blocks collection without ever
	// touching meta.max_block (the cold-start exit path is paused during
	// replays), so trusting meta alone can resume from a stale height and
	// idempotently re-fetch millions of blocks. The blocks collection is the
	// source of truth for what has actually landed.
	maxBlock, err := mongoClient.GetMaxBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to read max block: %w", err)
	}
	blocksMax, err := highestBlock(ctx, mongoClient)
	if err != nil {
		return fmt.Errorf("failed to read highest block: %w", err)
	}
	if blocksMax > maxBlock {
		log.Printf("[LiveSync] meta.max_block (%d) is behind the blocks collection (%d); resuming from the latter",
			maxBlock, blocksMax)
		maxBlock = blocksMax
	}
	nextBlock := int64(maxBlock) + 1
	log.Printf("[LiveSync] Starting from block %d (chunk=%d, follow_threshold=%d)",
		nextBlock, chunkSize, followThreshold)

	var head int64
	headAt := time.Time{}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Refresh the chain head periodically (and once at startup).
		if headAt.IsZero() || time.Since(headAt) > headRefreshInterval {
			props, err := api.GetDynamicGlobalProperties(ctx)
			if err != nil {
				log.Printf("[LiveSync] Failed to get chain head: %v (retrying)", err)
				time.Sleep(retrySleep)
				continue
			}
			head = int64(props.HeadBlockNumber)
			headAt = time.Now()
		}

		if head-nextBlock+1 > int64(followThreshold) {
			end := nextBlock + int64(chunkSize)
			if end > head+1 {
				end = head + 1
			}
			processed, err := syncChunk(ctx, api, mongoClient, nextBlock, end)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				log.Printf("[LiveSync] Chunk %d-%d failed: %v (retrying)", nextBlock, end-1, err)
				time.Sleep(retrySleep)
				continue
			}
			nextBlock = processed
			continue
		}

		processed, err := followBlock(ctx, api, mongoClient, nextBlock)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			if errors.Is(err, sdkv2.ErrBlockNotFound) {
				// Chain head reached — wait for new blocks.
				time.Sleep(followPollInterval)
				headAt = time.Time{} // force head refresh on next loop
				continue
			}
			log.Printf("[LiveSync] Block %d failed: %v (retrying)", nextBlock, err)
			time.Sleep(retrySleep)
			continue
		}
		nextBlock = processed
	}
}

// syncChunk fetches one chunk [from, to) via the SDK v2 range methods —
// block headers, all ops, and virtual ops run concurrently because the ops
// calls only need block numbers — then converts and upserts everything and
// commits meta.max_block. On any leg failure the whole chunk is retried by
// the caller (idempotent). Returns the next block number on success.
func syncChunk(ctx context.Context, api *sdkv2.API, mongoClient *mongo.Client, from, to int64) (int64, error) {
	fetchStart := time.Now()
	var (
		blocks  []*sdkv2.WrapBlock
		opsAll  map[uint][]*protocol.OperationObject
		opsVirt map[uint][]*protocol.OperationObject
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		blocks, err = api.GetBlocks(gctx, uint(from), uint(to))
		return err
	})
	g.Go(func() error {
		var err error
		opsAll, err = api.GetOpsInBlocks(gctx, uint(from), uint(to), false)
		return err
	})
	g.Go(func() error {
		var err error
		opsVirt, err = api.GetOpsInBlocks(gctx, uint(from), uint(to), true)
		return err
	})
	if err := g.Wait(); err != nil {
		return from, fmt.Errorf("range fetch: %w", err)
	}

	modelBlocks := make([]*model.Block, 0, len(blocks))
	var modelTxs []*model.Transaction
	var modelOps []*model.Operation

	for _, wb := range blocks {
		modelBlock, err := rpc.ConvertBlock(wb.Block, uint32(wb.BlockNum))
		if err != nil {
			return from, fmt.Errorf("convert block %d: %w", wb.BlockNum, err)
		}
		modelBlocks = append(modelBlocks, modelBlock)

		for i := range wb.Block.Transactions {
			modelTx, err := rpc.ConvertTransaction(&wb.Block.Transactions[i], uint32(wb.BlockNum), int32(i))
			if err != nil {
				log.Printf("[LiveSync] Convert transaction %d in block %d failed: %v", i, wb.BlockNum, err)
				continue
			}
			modelTxs = append(modelTxs, modelTx)
		}

		modelOps = append(modelOps, rpc.ConvertBlockOps(uint32(wb.BlockNum), opsAll[wb.BlockNum], opsVirt[wb.BlockNum])...)
	}

	// Write order (crash-consistency invariant): operations and transactions
	// FIRST, block headers LAST. The startup resume point is the highest block
	// in the blocks collection, so a block header must never land before its
	// operations — a crash in between would otherwise leave a permanent ops
	// hole that the resume logic skips past.
	writeStart := time.Now()
	if err := upsertOpsInBatches(ctx, mongoClient, modelOps); err != nil {
		return from, fmt.Errorf("upsert operations: %w", err)
	}
	if len(modelTxs) > 0 {
		if err := mongoClient.BulkUpsertTransactions(ctx, modelTxs); err != nil {
			return from, fmt.Errorf("upsert transactions: %w", err)
		}
	}
	if err := mongoClient.BulkUpsertBlocks(ctx, modelBlocks); err != nil {
		return from, fmt.Errorf("upsert blocks: %w", err)
	}

	// Commit point of the chunk.
	if err := mongoClient.UpdateMaxBlock(ctx, uint32(to-1)); err != nil {
		return from, fmt.Errorf("update max block: %w", err)
	}
	metrics.UpdateCurrentBlock(uint32(to - 1))
	log.Printf("[LiveSync] Batch %d-%d: %d blocks, %d txs, %d ops (fetch %s, write %s)",
		from, to-1, len(modelBlocks), len(modelTxs), len(modelOps),
		writeStart.Sub(fetchStart).Round(time.Millisecond), time.Since(writeStart).Round(time.Millisecond))
	return to, nil
}

// upsertOpsInBatches writes operations in bounded sub-batches. A dense-era
// 256-block chunk carries ~20k operations; one giant BulkWrite on the
// six-index operations collection can run for minutes on a cold cache, with
// no progress visibility and a long crash-redo window.
func upsertOpsInBatches(ctx context.Context, mongoClient *mongo.Client, ops []*model.Operation) error {
	const batchSize = 2000
	for i := 0; i < len(ops); i += batchSize {
		end := i + batchSize
		if end > len(ops) {
			end = len(ops)
		}
		if err := mongoClient.BulkUpsertOperations(ctx, ops[i:end]); err != nil {
			return fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
	}
	return nil
}

// followBlock fetches a single block near the chain head, upserts it, and
// commits meta.max_block. ErrBlockNotFound means the chain head is reached.
func followBlock(ctx context.Context, api *sdkv2.API, mongoClient *mongo.Client, blockNum int64) (int64, error) {
	block, err := api.GetBlock(ctx, uint(blockNum))
	if err != nil {
		return blockNum, err
	}
	opsAll, err := api.GetOpsInBlock(ctx, uint(blockNum), false)
	if err != nil {
		return blockNum, err
	}
	opsVirt, err := api.GetOpsInBlock(ctx, uint(blockNum), true)
	if err != nil {
		return blockNum, err
	}

	modelBlock, err := rpc.ConvertBlock(block, uint32(blockNum))
	if err != nil {
		return blockNum, fmt.Errorf("convert block %d: %w", blockNum, err)
	}
	var modelTxs []*model.Transaction
	for i := range block.Transactions {
		modelTx, err := rpc.ConvertTransaction(&block.Transactions[i], uint32(blockNum), int32(i))
		if err != nil {
			log.Printf("[LiveSync] Convert transaction %d in block %d failed: %v", i, blockNum, err)
			continue
		}
		modelTxs = append(modelTxs, modelTx)
	}
	modelOps := rpc.ConvertBlockOps(uint32(blockNum), opsAll, opsVirt)

	// Same crash-consistency order as syncChunk: ops/txs before the block header.
	if len(modelOps) > 0 {
		if err := mongoClient.BulkUpsertOperations(ctx, modelOps); err != nil {
			return blockNum, fmt.Errorf("upsert operations: %w", err)
		}
	}
	if len(modelTxs) > 0 {
		if err := mongoClient.BulkUpsertTransactions(ctx, modelTxs); err != nil {
			return blockNum, fmt.Errorf("upsert transactions: %w", err)
		}
	}
	if err := mongoClient.BulkUpsertBlocks(ctx, []*model.Block{modelBlock}); err != nil {
		return blockNum, fmt.Errorf("upsert block: %w", err)
	}

	if err := mongoClient.UpdateMaxBlock(ctx, uint32(blockNum)); err != nil {
		return blockNum, fmt.Errorf("update max block: %w", err)
	}
	metrics.UpdateCurrentBlock(uint32(blockNum))
	return blockNum + 1, nil
}

// convertBlockOps used to live here; the shared implementation (op id
// conventions matching the ingest plugin) is rpc.ConvertBlockOps — reuse it
// in every RPC-based ingest path instead of re-deriving coordinates.

// highestBlock returns the highest block number present in the blocks
// collection (the _id is the block number; indexed, so this is instant even
// at 20M+ documents).
func highestBlock(ctx context.Context, mongoClient *mongo.Client) (uint32, error) {
	var doc struct {
		ID uint32 `bson:"_id"`
	}
	err := mongoClient.Database().Collection("blocks").FindOne(ctx,
		bson.M{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})).Decode(&doc)
	if errors.Is(err, drivermongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return doc.ID, nil
}
