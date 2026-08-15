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
	"github.com/steemit/steemdb-sync/internal/refresher"
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

	if !cfg.Refresher.Enabled {
		log.Fatal("Refresher is disabled in config (refresher.enabled=false)")
	}

	// Initialize MongoDB client
	mongoClient, err := mongo.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB client: %v", err)
	}
	defer mongoClient.Close(context.Background())

	// Initialize RPC client
	rpcTimeout, err := cfg.RPCTimeout()
	if err != nil {
		log.Fatalf("Invalid RPC timeout: %v", err)
	}
	rpcClient := rpc.NewClient(cfg.RPC.Endpoint, cfg.RPC.MaxRetry, rpcTimeout)

	// Start metrics HTTP server (port 9093; processor uses 9092)
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		metricsServer := &http.Server{
			Addr:    ":9093",
			Handler: metricsMux,
		}
		log.Printf("[Refresher] Starting metrics server on :9093/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Refresher] Metrics server error: %v", err)
		}
	}()

	// Graceful shutdown on signal
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		r := refresher.New(cfg, mongoClient, rpcClient)
		r.Run(ctx)
	}()

	<-sigChan
	log.Printf("[Refresher] Received signal, shutting down...")
	cancel()
	// Give tickers a moment to finish in-flight writes
	time.Sleep(5 * time.Second)

	log.Println("[Refresher] Done")
}
