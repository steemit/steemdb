package rpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// TestNewClient tests RPC client initialization
func TestNewClient(t *testing.T) {
	endpoint := "https://api.steemit.com"
	maxRetry := 3
	timeout := 30 * time.Second

	client := rpc.NewClient(endpoint, maxRetry, timeout)
	require.NotNil(t, client)

	// Verify client fields (if accessible)
	// Note: Since fields are not exported, we can only verify client is not nil
}

// TestNewClientWithInvalidEndpoint tests client creation with invalid endpoint
func TestNewClientWithInvalidEndpoint(t *testing.T) {
	// Even with invalid endpoint, client should be created
	// The error will occur when making actual RPC calls
	client := rpc.NewClient("invalid://endpoint", 3, 30*time.Second)
	assert.NotNil(t, client)
}

// TestGetBlockWithInvalidBlockNum tests GetBlock with invalid block number
// This test requires network access or will be skipped if RPC is unavailable
func TestGetBlockWithInvalidBlockNum(t *testing.T) {
	// Skip if network is not available
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client := rpc.NewClient("https://api.steemit.com", 3, 30*time.Second)
	ctx := context.Background()

	// Try to get a very high block number that doesn't exist
	block, err := client.GetBlock(ctx, 999999999)
	
	// Should return error for non-existent block
	assert.Error(t, err)
	assert.Nil(t, block)
}

// TestGetOpsInBlockWithInvalidBlockNum tests GetOpsInBlock with invalid block number
func TestGetOpsInBlockWithInvalidBlockNum(t *testing.T) {
	// Skip if network is not available
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client := rpc.NewClient("https://api.steemit.com", 3, 30*time.Second)
	ctx := context.Background()

	// Try to get operations for a very high block number
	ops, err := client.GetOpsInBlock(ctx, 999999999, false)
	
	// Should return error for non-existent block
	assert.Error(t, err)
	assert.Nil(t, ops)
}

// TestGetBlockWithOpsWithInvalidBlockNum tests GetBlockWithOps with invalid block number
func TestGetBlockWithOpsWithInvalidBlockNum(t *testing.T) {
	// Skip if network is not available
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client := rpc.NewClient("https://api.steemit.com", 3, 30*time.Second)
	ctx := context.Background()

	// Try to get block with ops for a very high block number
	block, regularOps, virtualOps, err := client.GetBlockWithOps(ctx, 999999999)
	
	// Should return error for non-existent block
	assert.Error(t, err)
	assert.Nil(t, block)
	assert.Nil(t, regularOps)
	assert.Nil(t, virtualOps)
}

// TestGetBlockWithOpsStructure tests the structure of GetBlockWithOps response
// This test requires network access and a valid block number
func TestGetBlockWithOpsStructure(t *testing.T) {
	// Skip if network is not available
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client := rpc.NewClient("https://api.steemit.com", 3, 30*time.Second)
	ctx := context.Background()

	// Use a known block number (e.g., block 1)
	blockNum := uint32(1)
	block, regularOps, virtualOps, err := client.GetBlockWithOps(ctx, blockNum)
	
	if err != nil {
		t.Skipf("Skipping test due to RPC error: %v", err)
		return
	}

	// Verify block is not nil
	require.NotNil(t, block)
	
	// Verify block has required fields
	assert.NotEmpty(t, block.BlockId, "Block should have BlockId")
	
	// Verify ops are not nil (may be empty)
	assert.NotNil(t, regularOps)
	assert.NotNil(t, virtualOps)
	
	// Verify regular ops are not virtual
	for _, op := range regularOps {
		assert.Equal(t, uint64(0), op.VirtualOperation, "Regular ops should have VirtualOperation = 0")
	}
	
	// Verify virtual ops are marked as virtual
	for _, op := range virtualOps {
		assert.Greater(t, op.VirtualOperation, uint64(0), "Virtual ops should have VirtualOperation > 0")
	}
}

// TestGetOpsInBlockOnlyVirtual tests GetOpsInBlock with onlyVirtual flag
func TestGetOpsInBlockOnlyVirtual(t *testing.T) {
	// Skip if network is not available
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	client := rpc.NewClient("https://api.steemit.com", 3, 30*time.Second)
	ctx := context.Background()

	// Use a known block number that likely has virtual ops
	blockNum := uint32(1000000) // A block that likely has virtual ops
	
	// Get all ops
	allOps, err := client.GetOpsInBlock(ctx, blockNum, false)
	if err != nil {
		t.Skipf("Skipping test due to RPC error: %v", err)
		return
	}
	
	// Get only virtual ops
	virtualOps, err := client.GetOpsInBlock(ctx, blockNum, true)
	require.NoError(t, err)
	
	// Verify all virtual ops are marked as virtual
	for _, op := range virtualOps {
		assert.Greater(t, op.VirtualOperation, uint64(0), "All returned ops should be virtual")
	}
	
	// Verify virtual ops count is less than or equal to all ops count
	assert.LessOrEqual(t, len(virtualOps), len(allOps), "Virtual ops should be subset of all ops")
}
