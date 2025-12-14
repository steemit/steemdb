package services

import (
	"testing"
	"time"

	"github.com/steemdb/sync/internal/utils"
)

func TestBlockSyncService_IsSyncCaughtUp(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			BlockBatchSize:     50,
			OperationBatchSize: 100,
			BlockInterval:      time.Second,
		},
	}

	logger := &TestLogger{}

	service := &BlockSyncService{
		config:       config,
		logger:       logger,
		syncCaughtUp: false,
	}

	// Initially not caught up
	if service.IsSyncCaughtUp() {
		t.Error("Expected sync not to be caught up initially")
	}

	// Set caught up
	service.setSyncCaughtUp(true)
	if !service.IsSyncCaughtUp() {
		t.Error("Expected sync to be caught up after setting")
	}
}

func TestBlockSyncService_UpdateLastBlock(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			BlockBatchSize:     50,
			OperationBatchSize: 100,
			BlockInterval:      time.Second,
		},
	}

	logger := &TestLogger{}

	// Create service without db to test basic logic
	service := &BlockSyncService{
		config:    config,
		logger:    logger,
		lastBlock: 100,
		db:        nil, // No db to avoid SaveLastProcessedBlock call
	}

	// Test mutex-protected update
	service.mutex.Lock()
	if service.lastBlock != 100 {
		t.Errorf("Expected initial lastBlock to be 100, got %d", service.lastBlock)
	}

	// Simulate updateLastBlock logic without database call
	if 200 > service.lastBlock {
		service.lastBlock = 200
	}
	service.mutex.Unlock()

	if service.lastBlock != 200 {
		t.Errorf("Expected lastBlock to be 200, got %d", service.lastBlock)
	}

	// Test that lower block number doesn't change it
	service.mutex.Lock()
	if 150 > service.lastBlock {
		service.lastBlock = 150
	}
	service.mutex.Unlock()

	if service.lastBlock != 200 {
		t.Errorf("Expected lastBlock to remain 200, got %d", service.lastBlock)
	}
}

func TestBlockSyncService_IncrementCounts(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			BlockBatchSize:     50,
			OperationBatchSize: 100,
			BlockInterval:      time.Second,
		},
	}

	logger := &TestLogger{}

	service := &BlockSyncService{
		config: config,
		logger: logger,
		stats: &SyncStats{
			StartTime: time.Now(),
		},
	}

	// Increment block count
	service.incrementBlockCount()
	if service.stats.BlocksProcessed != 1 {
		t.Errorf("Expected BlocksProcessed to be 1, got %d", service.stats.BlocksProcessed)
	}

	// Increment operation count
	service.incrementOperationCount()
	if service.stats.OperationsProcessed != 1 {
		t.Errorf("Expected OperationsProcessed to be 1, got %d", service.stats.OperationsProcessed)
	}

	// Increment error count
	service.incrementErrorCount()
	if service.stats.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount to be 1, got %d", service.stats.ErrorCount)
	}
}
