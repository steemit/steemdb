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
	defaultChunkSize       = 100
	defaultFollowThreshold = 100
	defaultConcurrency     = 32
	followPollInterval     = 3 * time.Second
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

// run is the main loop: a single batching rule over the last irreversible
// block (LIB). Gaps beyond the follow threshold are processed in chunkSize
// batches; a gap within the threshold is fetched in one call; caught up means
// sleep and poll again. LIB targeting makes every ingested block final and
// guarantees its account_history entry exists. The commit point of a batch is
// meta.max_block; all writes are idempotent upserts, so a crash simply
// replays the uncommitted chunk.
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

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Refresh the chain target every loop: the target is the last
		// irreversible block, so every block we ingest is final and its
		// account_history entry exists (both the orphan-block and the
		// fresh-block empty-ops failure modes require following too close
		// to the head — LIB distance eliminates both).
		props, err := api.GetDynamicGlobalProperties(ctx)
		if err != nil {
			log.Printf("[LiveSync] Failed to get chain state: %v (retrying)", err)
			time.Sleep(retrySleep)
			continue
		}
		lib := int64(props.LastIrreversibleBlockNum)

		gap := lib - nextBlock + 1
		if gap <= 0 {
			// Caught up to LIB — wait for the next block(s). Typically the
			// next iteration fetches just 1-2 blocks.
			time.Sleep(followPollInterval)
			continue
		}

		// One batching rule for all distances: gaps beyond the threshold are
		// processed in chunkSize batches; a gap within the threshold is
		// fetched in a single call.
		end := lib + 1
		if gap > int64(followThreshold) {
			end = nextBlock + int64(chunkSize)
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

		// Guard against empty ops from account_history: every block carries
		// at least producer_reward, so an empty ops list means the node's AH
		// index has not recorded the block yet — fail the chunk and retry
		// instead of writing a header whose operations would never exist
		// (the 19-day ops-gap incident). Never triggers at LIB distance,
		// where AH indexing is complete by construction.
		if len(opsAll[wb.BlockNum]) == 0 && len(opsVirt[wb.BlockNum]) == 0 {
			return from, fmt.Errorf("block %d returned no ops (account_history not yet indexed; retrying)", wb.BlockNum)
		}

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
