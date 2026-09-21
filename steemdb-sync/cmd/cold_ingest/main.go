package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/metrics"
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
	http.HandleFunc("/ingest/applied_ops", handler.HandleAppliedOps)

	// Add metrics endpoint
	http.Handle("/metrics", metrics.Handler())

	// Start TPS calculator
	metrics.StartTPSCalculator(1 * time.Second)

	// Start HTTP server
	server := &http.Server{
		Addr:    cfg.Ingest.ListenAddr,
		Handler: http.DefaultServeMux,
	}

	// The ingest endpoint has no authentication: anyone who can reach it
	// can write arbitrary operations into the database. Warn loudly when
	// it is bound to something other than loopback.
	if addr := cfg.Ingest.ListenAddr; !isLoopbackListenAddr(addr) {
		log.Printf("WARNING: ingest server listening on %q (non-loopback). "+
			"The ingest endpoint is unauthenticated; make sure this address is "+
			"only reachable from a trusted network (e.g. a docker network shared "+
			"with steemd), never exposed to the host LAN or public internet.", addr)
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

// isLoopbackListenAddr reports whether the listen address only accepts
// connections from the local machine. Empty means http.DefaultAddr
// (":8080", all interfaces); a leading ":" or "0.0.0.0" host means all
// interfaces; a host part of "localhost" or "127.x.x.x" is loopback.
func isLoopbackListenAddr(addr string) bool {
	if addr == "" {
		return false
	}
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		// ":8080" style — listens on all interfaces.
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
