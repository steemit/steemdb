package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/steemit/steemutil/protocol"
	protocolapi "github.com/steemit/steemutil/protocol/api"

	"github.com/steemit/steemdb-sync/internal/checker"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// rpcFetcher is the RPC surface repair needs: the block header plus the two
// op listings per block (all ops, virtual-only ops).
type rpcFetcher interface {
	GetBlock(ctx context.Context, blockNum uint32) (*protocolapi.Block, error)
	GetOpsInBlock(ctx context.Context, blockNum uint32, onlyVirtual bool) ([]*protocol.OperationObject, error)
}

// blockWriter is the persistence surface repair needs. The ORDER of the
// write calls inside repairBlock is the crash-consistency contract:
// operations → transactions → block header → max_block, identical to
// cmd/live_sync. A block header must never land before its operations —
// the scanner and live_sync's resume logic both treat the header as "this
// block is done", so a header-first write would hide a partial ops failure
// from every gap-detection path.
type blockWriter interface {
	BulkUpsertOperations(ctx context.Context, ops []*model.Operation) error
	BulkUpsertTransactions(ctx context.Context, txs []*model.Transaction) error
	BulkUpsertBlocks(ctx context.Context, blocks []*model.Block) error
	GetMaxBlock(ctx context.Context) (uint32, error)
	UpdateMaxBlock(ctx context.Context, blockNum uint32) error
}

func main() {
	var (
		configPath string
		mode       string
		startBlock uint64
		endBlock   uint64
		dryRun     bool
	)
	flag.StringVar(&configPath, "config", "configs/config.yaml", "Path to configuration file")
	flag.StringVar(&mode, "mode", "blocks", "Repair mode: blocks (missing block repair) or backfill-accounts (populate operations.accounts)")
	flag.Uint64Var(&startBlock, "start", 0, "Start block number (0 = from block 1)")
	flag.Uint64Var(&endBlock, "end", 0, "End block number (0 = use max_block from meta)")
	flag.BoolVar(&dryRun, "dry-run", false, "Dry run mode (scan only, don't repair)")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize MongoDB client
	mongoClient, err := mongo.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB client: %v", err)
	}
	defer mongoClient.Close(context.Background())

	ctx := context.Background()

	// Backfill mode: one-time migration that populates operations.accounts
	// for data ingested before the field existed.
	if mode == "backfill-accounts" {
		log.Printf("Backfilling operations.accounts...")
		updated, err := mongoClient.BackfillOperationAccounts(ctx, 1000)
		if err != nil {
			log.Fatalf("Backfill failed after %d updates: %v", updated, err)
		}
		log.Printf("Backfill complete: %d operations updated", updated)
		return
	}
	if mode != "blocks" {
		log.Fatalf("Unknown mode: %s (supported: blocks, backfill-accounts)", mode)
	}

	// Determine scan range
	var scanStart, scanEnd uint32

	if startBlock > 0 {
		scanStart = uint32(startBlock)
	} else {
		scanStart = 1
	}

	if endBlock > 0 {
		scanEnd = uint32(endBlock)
	} else {
		// Get max block from meta
		maxBlock, err := mongoClient.GetMaxBlock(ctx)
		if err != nil {
			log.Fatalf("Failed to get max block: %v", err)
		}
		scanEnd = maxBlock
	}

	if scanStart > scanEnd {
		log.Fatalf("Invalid range: start (%d) > end (%d)", scanStart, scanEnd)
	}

	log.Printf("Repair tool starting...")
	log.Printf("Scan range: %d - %d", scanStart, scanEnd)
	if dryRun {
		log.Printf("Mode: DRY RUN (scan only)")
	}

	// Step 1: Scan for missing blocks
	scanner := checker.NewScanner(mongoClient)
	scanResult, err := scanner.Scan(ctx, scanEnd)
	if err != nil {
		log.Fatalf("Failed to scan blocks: %v", err)
	}

	// Filter missing blocks within scan range
	filteredMissing := make([]checker.MissingBlock, 0)
	for _, mb := range scanResult.MissingBlocks {
		if mb.BlockNum >= scanStart && mb.BlockNum <= scanEnd {
			filteredMissing = append(filteredMissing, mb)
		}
	}

	log.Printf("Scan complete: %d blocks scanned, %d missing blocks found", scanResult.TotalScanned, len(filteredMissing))

	if len(filteredMissing) == 0 {
		log.Println("No missing blocks found. Database is complete.")
		return
	}

	// Step 2: Merge consecutive blocks into ranges
	ranges := scanner.MergeRanges(filteredMissing)
	log.Printf("Missing blocks merged into %d ranges:", len(ranges))
	for _, r := range ranges {
		log.Printf("  Range: %s (%d blocks)", r.String(), r.Count())
	}

	if dryRun {
		log.Println("Dry run mode: exiting without repair")
		return
	}

	// Step 3: Repair missing blocks
	log.Println("Starting repair...")

	// Get RPC timeout
	rpcTimeout, err := cfg.RPCTimeout()
	if err != nil {
		log.Fatalf("Invalid RPC timeout: %v", err)
	}

	// Initialize RPC client
	rpcClient := rpc.NewClient(cfg.RPC.Endpoint, cfg.RPC.MaxRetry, rpcTimeout)

	// Repair each missing block
	repaired := 0
	failed := 0

	for _, mb := range filteredMissing {
		log.Printf("Repairing block %d (reason: %s)...", mb.BlockNum, mb.Reason)

		txCount, opCount, err := repairBlock(ctx, rpcClient, mongoClient, mb.BlockNum)
		if err != nil {
			log.Printf("Failed to repair block %d: %v", mb.BlockNum, err)
			failed++
			continue
		}

		repaired++
		log.Printf("Block %d repaired successfully (%d transactions, %d operations)",
			mb.BlockNum, txCount, opCount)

		// Small delay to avoid overwhelming the RPC node
		time.Sleep(100 * time.Millisecond)
	}

	// Summary
	log.Println("\n=== Repair Summary ===")
	log.Printf("Total missing blocks: %d", len(filteredMissing))
	log.Printf("Successfully repaired: %d", repaired)
	log.Printf("Failed: %d", failed)
	log.Printf("Success rate: %.2f%%", float64(repaired)/float64(len(filteredMissing))*100)
}

// repairBlock re-fetches one block from RPC and re-writes its raw-layer
// documents. Idempotent (all writes are upserts), so re-running repair after
// a partial failure is safe.
func repairBlock(ctx context.Context, fetcher rpcFetcher, writer blockWriter, blockNum uint32) (txCount, opCount int, err error) {
	// Fetch the same two listings live_sync consumes: the all-ops list
	// (regular ops at collapsed op_in_trx coordinates, virtual ops
	// interleaved) and the dedicated virtual-ops list. Coordinate
	// normalization happens in rpc.ConvertBlockOps — the shared
	// implementation of the plugin's op id convention.
	block, err := fetcher.GetBlock(ctx, blockNum)
	if err != nil {
		return 0, 0, fmt.Errorf("get block: %w", err)
	}
	allOps, err := fetcher.GetOpsInBlock(ctx, blockNum, false)
	if err != nil {
		return 0, 0, fmt.Errorf("get ops: %w", err)
	}
	virtualOps, err := fetcher.GetOpsInBlock(ctx, blockNum, true)
	if err != nil {
		return 0, 0, fmt.Errorf("get virtual ops: %w", err)
	}

	// Convert block
	modelBlock, err := rpc.ConvertBlock(block, blockNum)
	if err != nil {
		return 0, 0, fmt.Errorf("convert block: %w", err)
	}

	// Convert transactions
	var modelTxs []*model.Transaction
	for i := range block.Transactions {
		modelTx, err := rpc.ConvertTransaction(&block.Transactions[i], blockNum, int32(i))
		if err != nil {
			log.Printf("Failed to convert transaction %d in block %d: %v", i, blockNum, err)
			continue
		}
		modelTxs = append(modelTxs, modelTx)
	}

	// Convert operations with renumbered coordinates (multi-op transactions
	// are re-numbered by array position; virtual ops get the plugin's
	// trx=-1, op=1..n coordinates), producing the same `_id`s live_sync and
	// the ingest plugin write.
	modelOps := rpc.ConvertBlockOps(blockNum, allOps, virtualOps)

	// Write order (crash-consistency invariant, same as cmd/live_sync):
	// operations and transactions FIRST, block header LAST.
	if len(modelOps) > 0 {
		if err := writer.BulkUpsertOperations(ctx, modelOps); err != nil {
			return 0, 0, fmt.Errorf("upsert operations: %w", err)
		}
	}
	if len(modelTxs) > 0 {
		if err := writer.BulkUpsertTransactions(ctx, modelTxs); err != nil {
			return 0, 0, fmt.Errorf("upsert transactions: %w", err)
		}
	}
	if err := writer.BulkUpsertBlocks(ctx, []*model.Block{modelBlock}); err != nil {
		return 0, 0, fmt.Errorf("upsert block: %w", err)
	}

	// Update max block if this is the highest block.
	currentMax, err := writer.GetMaxBlock(ctx)
	if err != nil {
		log.Printf("Failed to read max block: %v", err)
	} else if blockNum > currentMax {
		if err := writer.UpdateMaxBlock(ctx, blockNum); err != nil {
			log.Printf("Failed to update max block: %v", err)
		}
	}

	return len(modelTxs), len(modelOps), nil
}
