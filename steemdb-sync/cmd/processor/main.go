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
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/processor"
	"github.com/steemit/steemdb-sync/internal/processor/handlers"
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

	if !cfg.Processor.Enabled {
		log.Fatal("Processor is disabled in config (processor.enabled=false)")
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

	// Initialize RPC client (needed for comment handler in Batch 2 and account refresher in Batch 3)
	rpcClient := rpc.NewClient(cfg.RPC.Endpoint, cfg.RPC.MaxRetry, rpcTimeout)

	// Build shared processor context
	pctx := &processor.Context{
		Cfg:         cfg,
		MongoClient: mongoClient,
		RPCClient:   rpcClient,
	}

	// Create dispatcher and register handlers
	dispatcher := processor.NewDispatcher(pctx)
	inserter := handlers.NewMongoInserter(mongoClient.Database())

	// Batch 1 handlers
	dispatcher.Register("vote", handlers.NewVoteHandler(inserter))
	dispatcher.Register("transfer", handlers.NewTransferHandler(inserter))
	dispatcher.Register("curation_reward", handlers.NewCurationRewardHandler(inserter))
	dispatcher.Register("author_reward", handlers.NewAuthorRewardHandler(inserter))

	// Batch 2 handlers (comment — phase 1: op data only, no get_content RPC)
	dispatcher.Register("comment", handlers.NewCommentHandler(inserter))
	dispatcher.Register("comment_options", handlers.NewCommentOptionsHandler(inserter))

	log.Printf("[Processor] Registered handlers for op_types: %v", dispatcher.RegisteredTypes())

	// Create processor
	proc, err := processor.NewProcessor(pctx, dispatcher)
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}

	// Start metrics HTTP server (port 9092)
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		metricsServer := &http.Server{
			Addr:    ":9092",
			Handler: metricsMux,
		}
		log.Printf("[Processor] Starting metrics server on :9092/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Processor] Metrics server error: %v", err)
		}
	}()

	// Graceful shutdown on signal
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run processor in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- proc.Run(ctx)
	}()

	// Wait for signal or processor exit
	select {
	case sig := <-sigChan:
		log.Printf("[Processor] Received signal %v, shutting down...", sig)
		cancel()
		// Give processor time to finish current block
		select {
		case <-time.After(30 * time.Second):
			log.Printf("[Processor] Shutdown timeout exceeded, forcing exit")
		case err := <-errChan:
			log.Printf("[Processor] Shutdown complete (err=%v)", err)
		}
	case err := <-errChan:
		log.Printf("[Processor] Processor exited: %v", err)
	}

	log.Println("[Processor] Done")
}
