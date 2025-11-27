package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/services"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
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

	logger.Info("Starting SteemDB Sync Service",
		utils.String("version", "1.0.0"),
		utils.String("config", configPath),
	)

	// Initialize database
	db, err := database.NewMongoDB(cfg.MongoDB, logger)
	if err != nil {
		logger.Fatal("Failed to connect to MongoDB", utils.Error(err))
	}

	// Initialize Steem client
	steemClient := steem.NewClient(cfg.Steem.Nodes, logger)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create indexes
	if err := db.CreateIndexes(ctx); err != nil {
		logger.Error("Failed to create database indexes", utils.Error(err))
	}

	// Initialize services
	serviceManager := services.NewManager(cfg, db, steemClient, logger)

	// Start services
	var wg sync.WaitGroup

	// Start block sync service
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting block sync service")
		if err := serviceManager.BlockSync.Start(ctx); err != nil {
			logger.Error("Block sync service error", utils.Error(err))
		}
	}()

	// Start history service
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting history service")
		if err := serviceManager.History.Start(ctx); err != nil {
			logger.Error("History service error", utils.Error(err))
		}
	}()

	// Start witnesses service
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting witnesses service")
		if err := serviceManager.Witnesses.Start(ctx); err != nil {
			logger.Error("Witnesses service error", utils.Error(err))
		}
	}()

	// Start metrics server if enabled
	if cfg.Metrics.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("Starting metrics server", utils.Int("port", cfg.Metrics.Port))
			if err := serviceManager.StartMetricsServer(ctx); err != nil {
				logger.Error("Metrics server error", utils.Error(err))
			}
		}()
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("SteemDB Sync Service started successfully")
	<-sigChan

	logger.Info("Shutting down SteemDB Sync Service...")
	cancel()

	// Wait for all services to stop with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All services stopped gracefully")
	case <-time.After(30 * time.Second):
		logger.Warn("Force shutdown after timeout")
	}

	// Close database connection
	if err := db.Close(context.Background()); err != nil {
		logger.Error("Failed to close database connection", utils.Error(err))
	}

	logger.Info("SteemDB Sync Service stopped")
}
