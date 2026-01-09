package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.yaml", "Path to configuration file")
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

	// Get RPC timeout
	rpcTimeout, err := cfg.RPCTimeout()
	if err != nil {
		log.Fatalf("Invalid RPC timeout: %v", err)
	}

	// Initialize RPC client
	rpcClient := rpc.NewClient(cfg.RPC.Endpoint, cfg.RPC.MaxRetry, rpcTimeout)

	// Get starting block number
	ctx := context.Background()
	startBlock, err := mongoClient.GetMaxBlock(ctx)
	if err != nil {
		log.Fatalf("Failed to get max block: %v", err)
	}

	nextBlock := startBlock + 1
	log.Printf("Starting live sync from block %d", nextBlock)

	// Start metrics HTTP server (optional, for monitoring)
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		metricsServer := &http.Server{
			Addr:    ":9091",
			Handler: metricsMux,
		}
		log.Printf("Starting metrics server on :9091/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Main sync loop
	for {
		select {
		case <-sigChan:
			log.Println("Shutting down...")
			return
		default:
			// Try to get block
			block, regularOps, virtualOps, err := rpcClient.GetBlockWithOps(ctx, nextBlock)
			if err != nil {
				log.Printf("Failed to get block %d: %v, retrying in 5s...", nextBlock, err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Convert block
			modelBlock, err := rpc.ConvertBlock(block, nextBlock)
			if err != nil {
				log.Printf("Failed to convert block %d: %v", nextBlock, err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Convert transactions
			var modelTxs []*model.Transaction
			for i, trx := range block.Transactions {
				trxPtr := &trx
				modelTx, err := rpc.ConvertTransaction(trxPtr, nextBlock, int32(i))
				if err != nil {
					log.Printf("Failed to convert transaction %d in block %d: %v", i, nextBlock, err)
					continue
				}
				modelTxs = append(modelTxs, modelTx)
			}

			// Convert operations
			var modelOps []*model.Operation
			for _, opObj := range regularOps {
				modelOp, err := rpc.ConvertOperation(opObj, "rpc")
				if err != nil {
					log.Printf("Failed to convert operation in block %d: %v", nextBlock, err)
					continue
				}
				modelOps = append(modelOps, modelOp)
			}
			for _, opObj := range virtualOps {
				modelOp, err := rpc.ConvertOperation(opObj, "rpc")
				if err != nil {
					log.Printf("Failed to convert virtual operation in block %d: %v", nextBlock, err)
					continue
				}
				modelOps = append(modelOps, modelOp)
			}

			// Write to MongoDB
			if err := mongoClient.BulkUpsertBlocks(ctx, []*model.Block{modelBlock}); err != nil {
				log.Printf("Failed to write block %d: %v", nextBlock, err)
				time.Sleep(5 * time.Second)
				continue
			}

			if len(modelTxs) > 0 {
				if err := mongoClient.BulkUpsertTransactions(ctx, modelTxs); err != nil {
					log.Printf("Failed to write transactions for block %d: %v", nextBlock, err)
					time.Sleep(5 * time.Second)
					continue
				}
			}

			if len(modelOps) > 0 {
				if err := mongoClient.BulkUpsertOperations(ctx, modelOps); err != nil {
					log.Printf("Failed to write operations for block %d: %v", nextBlock, err)
					time.Sleep(5 * time.Second)
					continue
				}
			}

			// Update max block
			if err := mongoClient.UpdateMaxBlock(ctx, nextBlock); err != nil {
				log.Printf("Failed to update max block: %v", err)
			}

			// Update metrics
			metrics.UpdateCurrentBlock(nextBlock)
			metrics.RecordIngestOp("rpc")
			
			log.Printf("Synced block %d (%d transactions, %d operations)", nextBlock, len(modelTxs), len(modelOps))
			nextBlock++

			// Small delay to avoid overwhelming the RPC node
			time.Sleep(100 * time.Millisecond)
		}
	}
}
