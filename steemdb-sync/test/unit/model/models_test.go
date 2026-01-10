package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/steemit/steemdb-sync/internal/model"
)

// TestOperationID tests Operation ID generation
func TestOperationID(t *testing.T) {
	// Normal operation
	id := model.OperationID(100, 2, 0)
	assert.Equal(t, "100:2:0", id)
	
	// Different block number
	id2 := model.OperationID(200, 2, 0)
	assert.Equal(t, "200:2:0", id2)
	assert.NotEqual(t, id, id2)
	
	// Different transaction index
	id3 := model.OperationID(100, 3, 0)
	assert.Equal(t, "100:3:0", id3)
	assert.NotEqual(t, id, id3)
	
	// Different operation index
	id4 := model.OperationID(100, 2, 1)
	assert.Equal(t, "100:2:1", id4)
	assert.NotEqual(t, id, id4)
}

// TestOperationIDVirtual tests Virtual operation ID generation
func TestOperationIDVirtual(t *testing.T) {
	// Virtual operation (trx_index = -1)
	idVirtual := model.OperationID(100, -1, -1)
	assert.Equal(t, "100:-1:-1", idVirtual)
	
	// Virtual operation with different op index
	idVirtual2 := model.OperationID(100, -1, 0)
	assert.Equal(t, "100:-1:0", idVirtual2)
}

// TestOperationIDUniqueness tests ID uniqueness
func TestOperationIDUniqueness(t *testing.T) {
	// Generate IDs for different operations
	ids := make(map[string]bool)
	
	for blockNum := uint32(1); blockNum <= 10; blockNum++ {
		for trxIndex := int32(0); trxIndex < 5; trxIndex++ {
			for opIndex := int32(0); opIndex < 3; opIndex++ {
				id := model.OperationID(blockNum, trxIndex, opIndex)
				// Verify ID is unique
				assert.False(t, ids[id], "Duplicate ID: %s", id)
				ids[id] = true
			}
		}
	}
	
	// Should have 10 * 5 * 3 = 150 unique IDs
	assert.Equal(t, 150, len(ids))
}

// TestOperationIDFormat tests ID format
func TestOperationIDFormat(t *testing.T) {
	id := model.OperationID(12345, 67, 89)
	
	// Verify format: block_num:trx_index:op_index
	assert.Contains(t, id, "12345")
	assert.Contains(t, id, "67")
	assert.Contains(t, id, "89")
	assert.Contains(t, id, ":")
}

// TestBlockSerialization tests Block struct serialization
func TestBlockSerialization(t *testing.T) {
	block := &model.Block{
		ID:               100,
		BlockNum:         100,
		BlockID:          "abc123",
		Previous:         "def456",
		Timestamp:        time.Now(),
		Witness:          "witness1",
		TransactionCount: 5,
	}
	
	// Verify all fields are set
	assert.Equal(t, uint32(100), block.ID)
	assert.Equal(t, uint32(100), block.BlockNum)
	assert.Equal(t, "abc123", block.BlockID)
	assert.Equal(t, "def456", block.Previous)
	assert.NotZero(t, block.Timestamp)
	assert.Equal(t, "witness1", block.Witness)
	assert.Equal(t, 5, block.TransactionCount)
}

// TestTransactionSerialization tests Transaction struct serialization
func TestTransactionSerialization(t *testing.T) {
	tx := &model.Transaction{
		ID:         "tx123",
		BlockNum:   100,
		TrxIndex:   2,
		Expiration: time.Now(),
	}
	
	// Verify all fields are set
	assert.Equal(t, "tx123", tx.ID)
	assert.Equal(t, uint32(100), tx.BlockNum)
	assert.Equal(t, int32(2), tx.TrxIndex)
	assert.NotZero(t, tx.Expiration)
}

// TestOperationSerialization tests Operation struct serialization
func TestOperationSerialization(t *testing.T) {
	op := &model.Operation{
		ID:       "100:2:0",
		BlockNum: 100,
		TrxID:    "tx123",
		TrxIndex: 2,
		OpIndex:  0,
		OpType:   "transfer",
		OpValue: map[string]interface{}{
			"from":   "alice",
			"to":     "bob",
			"amount": "1.000 STEEM",
		},
		Virtual: false,
		Source:  "plugin",
	}
	
	// Verify all fields are set
	assert.Equal(t, "100:2:0", op.ID)
	assert.Equal(t, uint32(100), op.BlockNum)
	assert.Equal(t, "tx123", op.TrxID)
	assert.Equal(t, int32(2), op.TrxIndex)
	assert.Equal(t, int32(0), op.OpIndex)
	assert.Equal(t, "transfer", op.OpType)
	assert.NotNil(t, op.OpValue)
	assert.Equal(t, false, op.Virtual)
	assert.Equal(t, "plugin", op.Source)
}

// TestOperationVirtual tests Virtual operation
func TestOperationVirtual(t *testing.T) {
	op := &model.Operation{
		ID:       "100:-1:-1",
		BlockNum: 100,
		TrxID:    "",
		TrxIndex: -1,
		OpIndex:  -1,
		OpType:   "author_reward",
		OpValue: map[string]interface{}{
			"author": "alice",
		},
		Virtual: true,
		Source:  "rpc",
	}
	
	assert.True(t, op.Virtual)
	assert.Equal(t, int32(-1), op.TrxIndex)
	assert.Equal(t, int32(-1), op.OpIndex)
	assert.Equal(t, "rpc", op.Source)
}

// TestMetaSerialization tests Meta struct serialization
func TestMetaSerialization(t *testing.T) {
	meta := &model.Meta{
		ID:            "sync_state",
		MaxBlock:      1000000,
		ColdStartDone: true,
		UpdatedAt:     time.Now(),
	}
	
	// Verify all fields are set
	assert.Equal(t, "sync_state", meta.ID)
	assert.Equal(t, uint32(1000000), meta.MaxBlock)
	assert.True(t, meta.ColdStartDone)
	assert.NotZero(t, meta.UpdatedAt)
}
