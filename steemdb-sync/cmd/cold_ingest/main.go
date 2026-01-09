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
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/pipeline"
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

	// Check if cold start is already done
	ctx := context.Background()
	maxBlock, err := mongoClient.GetMaxBlock(ctx)
	if err != nil {
		log.Fatalf("Failed to get max block: %v", err)
	}
	log.Printf("Current max block: %d", maxBlock)

	// Create batcher
	batcher, err := pipeline.NewBatcher(cfg, mongoClient)
	if err != nil {
		log.Fatalf("Failed to create batcher: %v", err)
	}

	// Start batcher
	batcher.Start()
	defer batcher.Stop()

	// Create HTTP handler
	handler := pipeline.NewIngestHandler(batcher)
	http.HandleFunc("/ingest/applied_op", handler.HandleAppliedOp)

	// Start HTTP server
	server := &http.Server{
		Addr:    cfg.Ingest.ListenAddr,
		Handler: http.DefaultServeMux,
	}

	go func() {
		log.Printf("Starting ingest server on %s", cfg.Ingest.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Check for target height
	targetHeight := cfg.ColdStart.TargetHeight
	if targetHeight > 0 {
		log.Printf("Cold start target height: %d (safety margin: %d)", targetHeight, cfg.ColdStart.SafetyMargin)
		
		// Monitor for target height
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					maxBlockSeen := batcher.GetMaxBlockSeen()
					if maxBlockSeen > 0 {
						log.Printf("Max block seen: %d (target: %d)", maxBlockSeen, targetHeight-cfg.ColdStart.SafetyMargin)
					}

					if maxBlockSeen >= targetHeight-cfg.ColdStart.SafetyMargin {
						log.Printf("Reached target height (seen: %d, target: %d), shutting down...", 
							maxBlockSeen, targetHeight-cfg.ColdStart.SafetyMargin)
						
						// Flush all batches
						if err := batcher.Stop(); err != nil {
							log.Printf("Error flushing batches: %v", err)
						}

						// Update meta
						if err := mongoClient.SetColdStartDone(ctx); err != nil {
							log.Printf("Error setting cold start done: %v", err)
						}

						// Update max block
						if err := mongoClient.UpdateMaxBlock(ctx, maxBlockSeen); err != nil {
							log.Printf("Error updating max block: %v", err)
						}

						log.Println("Cold start completed, exiting...")
						os.Exit(0)
					}

				case <-sigChan:
					return
				}
			}
		}()
	}

	// Wait for interrupt
	<-sigChan
	log.Println("Shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	// Flush remaining operations
	if err := batcher.Stop(); err != nil {
		log.Printf("Error flushing batches: %v", err)
	}

	log.Println("Shutdown complete")
}
