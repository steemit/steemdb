package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/steemit/steemdb-sync/internal/checker"
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

func main() {
	var (
		configPath string
		startBlock uint64
		endBlock   uint64
		dryRun     bool
	)
	flag.StringVar(&configPath, "config", "configs/config.yaml", "Path to configuration file")
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

		// Get block and operations from RPC
		block, regularOps, virtualOps, err := rpcClient.GetBlockWithOps(ctx, mb.BlockNum)
		if err != nil {
			log.Printf("Failed to get block %d from RPC: %v", mb.BlockNum, err)
			failed++
			continue
		}

		// Convert block
		modelBlock, err := rpc.ConvertBlock(block, mb.BlockNum)
		if err != nil {
			log.Printf("Failed to convert block %d: %v", mb.BlockNum, err)
			failed++
			continue
		}

		// Convert transactions
		var modelTxs []*model.Transaction
		for i, trx := range block.Transactions {
			trxPtr := &trx
			modelTx, err := rpc.ConvertTransaction(trxPtr, mb.BlockNum, int32(i))
			if err != nil {
				log.Printf("Failed to convert transaction %d in block %d: %v", i, mb.BlockNum, err)
				continue
			}
			modelTxs = append(modelTxs, modelTx)
		}

		// Convert operations
		var modelOps []*model.Operation
		for _, opObj := range regularOps {
			modelOp, err := rpc.ConvertOperation(opObj, "rpc")
			if err != nil {
				log.Printf("Failed to convert operation in block %d: %v", mb.BlockNum, err)
				continue
			}
			modelOps = append(modelOps, modelOp)
		}
		for _, opObj := range virtualOps {
			modelOp, err := rpc.ConvertOperation(opObj, "rpc")
			if err != nil {
				log.Printf("Failed to convert virtual operation in block %d: %v", mb.BlockNum, err)
				continue
			}
			modelOps = append(modelOps, modelOp)
		}

		// Write to MongoDB (idempotent)
		if err := mongoClient.BulkUpsertBlocks(ctx, []*model.Block{modelBlock}); err != nil {
			log.Printf("Failed to write block %d: %v", mb.BlockNum, err)
			failed++
			continue
		}

		if len(modelTxs) > 0 {
			if err := mongoClient.BulkUpsertTransactions(ctx, modelTxs); err != nil {
				log.Printf("Failed to write transactions for block %d: %v", mb.BlockNum, err)
				failed++
				continue
			}
		}

		if len(modelOps) > 0 {
			if err := mongoClient.BulkUpsertOperations(ctx, modelOps); err != nil {
				log.Printf("Failed to write operations for block %d: %v", mb.BlockNum, err)
				failed++
				continue
			}
		}

		// Update max block if this is the highest block
		currentMax, _ := mongoClient.GetMaxBlock(ctx)
		if mb.BlockNum > currentMax {
			if err := mongoClient.UpdateMaxBlock(ctx, mb.BlockNum); err != nil {
				log.Printf("Failed to update max block: %v", err)
			}
		}

		repaired++
		log.Printf("Block %d repaired successfully (%d transactions, %d operations)",
			mb.BlockNum, len(modelTxs), len(modelOps))

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
