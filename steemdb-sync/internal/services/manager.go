package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/utils"
)

// Manager manages all sync services
type Manager struct {
	config *utils.Config
	db     *database.MongoDB
	steem  *utils.SteemClient
	logger utils.Logger

	// Services
	BlockSync *BlockSyncService
	CronTab   *CronTabService
}

// NewManager creates a new service manager
func NewManager(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *utils.SteemClient,
	logger utils.Logger,
) *Manager {
	manager := &Manager{
		config: config,
		db:     db,
		steem:  steemClient,
		logger: logger,
	}

	// Initialize services
	manager.BlockSync = NewBlockSyncService(config, db, steemClient, logger)
	manager.CronTab = NewCronTabService(config, db, steemClient, logger, manager.BlockSync)

	return manager
}

// StartMetricsServer starts the Prometheus metrics server
func (m *Manager) StartMetricsServer(ctx context.Context) error {
	if !m.config.Metrics.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle(m.config.Metrics.Path, promhttp.Handler())
	mux.HandleFunc("/health", m.healthCheckHandler)
	mux.HandleFunc("/ready", m.readinessCheckHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", m.config.Metrics.Port),
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Metrics server failed", utils.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// healthCheckHandler handles health check requests
func (m *Manager) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":%d,"version":"1.0.0","services":{"block_sync":"running","crontab":"running"}}`, time.Now().Unix())
}

// readinessCheckHandler handles readiness check requests
func (m *Manager) readinessCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if all services are ready
	ready := true
	services := make(map[string]string)

	// Check database connection
	if err := m.db.Ping(context.Background()); err != nil {
		ready = false
		services["database"] = "not_ready"
	} else {
		services["database"] = "ready"
	}

	// Check Steem connection
	if _, err := m.steem.GetDynamicGlobalProperties(context.Background()); err != nil {
		ready = false
		services["steem_rpc"] = "not_ready"
	} else {
		services["steem_rpc"] = "ready"
	}

	services["block_sync"] = "ready"
	services["crontab"] = "ready"

	status := "ready"
	statusCode := http.StatusOK
	if !ready {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"status":"%s","timestamp":%d,"services":{"database":"%s","steem_rpc":"%s","block_sync":"ready","crontab":"ready"}}`,
		status, time.Now().Unix(), services["database"], services["steem_rpc"])
}
