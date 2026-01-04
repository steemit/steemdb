package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/sync"
	"github.com/steemit/steemdb/sync/internal/utils"
)

func main() {
	// Load configuration
	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := utils.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := utils.NewLogger(cfg.Log)
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting SteemDB Sync Service (Layer 1: Raw Operation Sync)",
		utils.String("version", "1.0.0"),
		utils.String("config", configPath),
	)

	// Initialize database
	db, err := database.NewMongoDB(cfg.MongoDB, logger)
	if err != nil {
		logger.Fatal("Failed to connect to MongoDB", utils.Error(err))
	}
	defer func() {
		if err := db.Close(context.Background()); err != nil {
			logger.Error("Failed to close database connection", utils.Error(err))
		}
	}()

	// Initialize Steem client (has retry and node switching logic)
	steemClient := utils.NewSteemClient(cfg.Steem.Nodes, logger)

	// Create Raw Syncer
	rawSyncer := sync.NewRawSyncer(steemClient, db, logger, cfg)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get sync state to determine start block
	syncState, err := db.GetSyncState(ctx)
	if err != nil {
		logger.Fatal("Failed to get sync state", utils.Error(err))
	}

	startBlock := cfg.Sync.StartBlock
	if syncState.LastBlock > 0 && syncState.LastBlock >= startBlock {
		startBlock = syncState.LastBlock + 1
		logger.Info("Resuming from last synced block",
			utils.Int64("start_block", startBlock),
			utils.Int64("last_block", syncState.LastBlock),
		)
	} else {
		logger.Info("Starting from configured block",
			utils.Int64("start_block", startBlock),
		)
	}

	// Start sync loop in a goroutine
	go func() {
		ticker := time.NewTicker(cfg.Sync.BlockInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Get current sync state to determine actual start block
				currentState, err := db.GetSyncState(ctx)
				if err != nil {
					logger.Error("Failed to get sync state", utils.Error(err))
					time.Sleep(5 * time.Second)
					continue
				}

				actualStartBlock := cfg.Sync.StartBlock
				if currentState.LastBlock > 0 && currentState.LastBlock >= cfg.Sync.StartBlock {
					actualStartBlock = currentState.LastBlock + 1
				}

				if err := rawSyncer.SyncBlocks(ctx, actualStartBlock); err != nil {
					logger.Error("Error syncing blocks", utils.Error(err))
					time.Sleep(5 * time.Second)
				}
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("SteemDB Sync Service (Layer 1) started successfully")
	<-sigChan

	logger.Info("Shutting down SteemDB Sync Service...")
	cancel()

	// Wait a bit for graceful shutdown
	time.Sleep(2 * time.Second)

	logger.Info("SteemDB Sync Service stopped")
}
