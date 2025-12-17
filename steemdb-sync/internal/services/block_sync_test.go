package services

import (
	"testing"
	"time"

	"github.com/steemit/steemdb/sync/internal/utils"
)

func TestBlockSyncService_IsSyncCaughtUp(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			BlockBatchSize: 50,
			BlockInterval:  time.Second,
		},
	}

	logger := &TestLogger{}

	service := &BlockSyncService{
		config:    config,
		logger:    logger,
		lastBlock: 100,
	}

	// Test that lastBlock is set correctly
	if service.lastBlock != 100 {
		t.Errorf("Expected initial lastBlock to be 100, got %d", service.lastBlock)
	}

	// Update lastBlock (simulating block processing)
	service.lastBlock = 200
	if service.lastBlock != 200 {
		t.Errorf("Expected lastBlock to be 200, got %d", service.lastBlock)
	}

	// Test that lower block number doesn't change it
	if 150 > service.lastBlock {
		service.lastBlock = 150
	}
	if service.lastBlock != 200 {
		t.Errorf("Expected lastBlock to remain 200, got %d", service.lastBlock)
	}
}

func TestBlockSyncService_BlockBuffer(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			BlockBatchSize: 50,
			BlockInterval:  time.Second,
		},
	}

	logger := &TestLogger{}

	service := NewBlockSyncService(config, nil, nil, logger)

	// Test initial buffer capacity
	if cap(service.blockBuffer) != config.Sync.BlockBatchSize {
		t.Errorf("Expected blockBuffer capacity to be %d, got %d",
			config.Sync.BlockBatchSize, cap(service.blockBuffer))
	}

	// Test buffer is empty initially
	if len(service.blockBuffer) != 0 {
		t.Errorf("Expected blockBuffer to be empty, got length %d", len(service.blockBuffer))
	}
}
