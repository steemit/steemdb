package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// Manager manages all sync services
type Manager struct {
	config    *utils.Config
	db        *database.MongoDB
	steem     *steem.Client
	logger    utils.Logger

	// Services
	BlockSync *BlockSyncService
	History   *HistoryService
	Witnesses *WitnessService
}

// NewManager creates a new service manager
func NewManager(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *steem.Client,
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
	manager.History = NewHistoryService(config, db, steemClient, logger)
	manager.Witnesses = NewWitnessService(config, db, steemClient, logger)

	return manager
}

// StartMetricsServer starts the Prometheus metrics server
func (m *Manager) StartMetricsServer(ctx context.Context) error {
	if !m.config.Metrics.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle(m.config.Metrics.Path, promhttp.Handler())

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
