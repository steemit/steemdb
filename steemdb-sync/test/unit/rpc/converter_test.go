package rpc_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protocolapi "github.com/steemit/steemutil/protocol/api"
	"github.com/steemit/steemutil/protocol"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// TestConvertBlock tests block conversion
func TestConvertBlock(t *testing.T) {
	blockNum := uint32(1000)
	blockID := "0000000000000000000000000000000000000000"
	previous := "0000000000000000000000000000000000000001"
	witness := "witness1"
	timestamp := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create protocol block
	protocolBlock := &protocolapi.Block{
		BlockId:  blockID,
		Previous: previous,
		Witness:   witness,
		Timestamp: &protocol.Time{
			Time: &timestamp,
		},
		Transactions: []protocolapi.Transaction{
			{TransactionId: "tx1"},
			{TransactionId: "tx2"},
		},
	}

	// Convert
	result, err := rpc.ConvertBlock(protocolBlock, blockNum)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify conversion
	assert.Equal(t, blockNum, result.ID)
	assert.Equal(t, blockNum, result.BlockNum)
	assert.Equal(t, blockID, result.BlockID)
	assert.Equal(t, previous, result.Previous)
	assert.Equal(t, timestamp, result.Timestamp)
	assert.Equal(t, witness, result.Witness)
	assert.Equal(t, 2, result.TransactionCount)
}

// TestConvertBlockNilTimestamp tests block conversion with nil timestamp
func TestConvertBlockNilTimestamp(t *testing.T) {
	blockNum := uint32(1000)
	protocolBlock := &protocolapi.Block{
		BlockId:     "block1",
		Previous:    "previous1",
		Witness:     "witness1",
		Timestamp:   nil,
		Transactions: []protocolapi.Transaction{},
	}

	// Convert
	result, err := rpc.ConvertBlock(protocolBlock, blockNum)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify timestamp is set (should be current time approximately)
	assert.NotZero(t, result.Timestamp)
	timeDiff := time.Since(result.Timestamp)
	assert.Less(t, timeDiff, 1*time.Second, "Timestamp should be recent")
}

// TestConvertBlockNilBlock tests block conversion with nil block
func TestConvertBlockNilBlock(t *testing.T) {
	result, err := rpc.ConvertBlock(nil, 1000)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "block is nil")
}

// TestConvertTransaction tests transaction conversion
func TestConvertTransaction(t *testing.T) {
	blockNum := uint32(1000)
	trxIndex := int32(0)
	trxID := "tx1234567890abcdef"
	expiration := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create protocol transaction
	protocolTrx := &protocolapi.Transaction{
		TransactionId: trxID,
		Expiration: &protocol.Time{
			Time: &expiration,
		},
	}

	// Convert
	result, err := rpc.ConvertTransaction(protocolTrx, blockNum, trxIndex)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify conversion
	assert.Equal(t, trxID, result.ID)
	assert.Equal(t, blockNum, result.BlockNum)
	assert.Equal(t, trxIndex, result.TrxIndex)
	assert.Equal(t, expiration, result.Expiration)
}

// TestConvertTransactionNilExpiration tests transaction conversion with nil expiration
func TestConvertTransactionNilExpiration(t *testing.T) {
	blockNum := uint32(1000)
	trxIndex := int32(0)
	trxID := "tx1234567890abcdef"

	protocolTrx := &protocolapi.Transaction{
		TransactionId: trxID,
		Expiration:    nil,
	}

	// Convert
	result, err := rpc.ConvertTransaction(protocolTrx, blockNum, trxIndex)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify expiration is set (should be current time approximately)
	assert.NotZero(t, result.Expiration)
	timeDiff := time.Since(result.Expiration)
	assert.Less(t, timeDiff, 1*time.Second, "Expiration should be recent")
}

// TestConvertTransactionNilTransaction tests transaction conversion with nil transaction
func TestConvertTransactionNilTransaction(t *testing.T) {
	result, err := rpc.ConvertTransaction(nil, 1000, 0)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "transaction is nil")
}

// TestConvertOperation tests operation conversion
func TestConvertOperation(t *testing.T) {
	blockNum := uint32(1000)
	trxInBlock := uint32(0)
	opInTrx := uint16(0)
	trxID := "tx1234567890abcdef"
	source := "rpc"

	// Create a simple transfer operation
	transferOp := &protocol.TransferOperation{
		From:   "alice",
		To:     "bob",
		Amount: "1.000 STEEM",
		Memo:   "",
	}

	// Create protocol operation object
	opObj := &protocol.OperationObject{
		BlockNumber:            blockNum,
		TransactionInBlock:     trxInBlock,
		OperationInTransaction: opInTrx,
		TransactionID:          trxID,
		VirtualOperation:       0, // Regular operation
		Operation:              transferOp,
	}

	// Convert
	result, err := rpc.ConvertOperation(opObj, source)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify conversion
	expectedOpID := model.OperationID(blockNum, int32(trxInBlock), int32(opInTrx))
	assert.Equal(t, expectedOpID, result.ID)
	assert.Equal(t, blockNum, result.BlockNum)
	assert.Equal(t, trxID, result.TrxID)
	assert.Equal(t, int32(trxInBlock), result.TrxIndex)
	assert.Equal(t, int32(opInTrx), result.OpIndex)
	assert.Equal(t, "transfer", result.OpType)
	assert.False(t, result.Virtual, "Should not be virtual")
	assert.Equal(t, source, result.Source)
	assert.NotNil(t, result.OpValue)
}

// TestConvertOperationVirtual tests virtual operation conversion
func TestConvertOperationVirtual(t *testing.T) {
	blockNum := uint32(1000)
	trxInBlock := uint32(0)
	opInTrx := uint16(0)
	source := "rpc"

	// Create virtual operation (author_reward)
	authorRewardOp := &protocol.CommentRewardOperation{
		Author:  "alice",
		Permlink: "test-post",
		Payout:   "1.000 STEEM",
	}

	// Create protocol operation object
	opObj := &protocol.OperationObject{
		BlockNumber:            blockNum,
		TransactionInBlock:     trxInBlock,
		OperationInTransaction: opInTrx,
		TransactionID:          "", // Virtual ops have no transaction ID
		VirtualOperation:       1,  // Virtual operation
		Operation:              authorRewardOp,
	}

	// Convert
	result, err := rpc.ConvertOperation(opObj, source)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify virtual flag
	assert.True(t, result.Virtual, "Should be virtual")
	assert.Equal(t, source, result.Source)
}

// TestConvertOperationNilOperation tests operation conversion with nil operation
func TestConvertOperationNilOperation(t *testing.T) {
	result, err := rpc.ConvertOperation(nil, "rpc")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "operation object is nil")
}

// TestConvertOperationSource tests that source field is correctly set
func TestConvertOperationSource(t *testing.T) {
	transferOp := &protocol.TransferOperation{
		From:   "alice",
		To:     "bob",
		Amount: "1.000 STEEM",
		Memo:   "",
	}

	opObj := &protocol.OperationObject{
		BlockNumber:            1000,
		TransactionInBlock:     0,
		OperationInTransaction:   0,
		TransactionID:          "tx1",
		VirtualOperation:       0,
		Operation:              transferOp,
	}

	// Test with "rpc" source
	result1, err := rpc.ConvertOperation(opObj, "rpc")
	require.NoError(t, err)
	assert.Equal(t, "rpc", result1.Source)

	// Test with "plugin" source
	result2, err := rpc.ConvertOperation(opObj, "plugin")
	require.NoError(t, err)
	assert.Equal(t, "plugin", result2.Source)
}

// TestConvertOperationIDGeneration tests operation ID generation
func TestConvertOperationIDGeneration(t *testing.T) {
	blockNum := uint32(1000)
	trxInBlock := uint32(5)
	opInTrx := uint16(3)

	transferOp := &protocol.TransferOperation{
		From:   "alice",
		To:     "bob",
		Amount: "1.000 STEEM",
		Memo:   "",
	}

	opObj := &protocol.OperationObject{
		BlockNumber:            blockNum,
		TransactionInBlock:     trxInBlock,
		OperationInTransaction: opInTrx,
		TransactionID:          "tx1",
		VirtualOperation:       0,
		Operation:              transferOp,
	}

	result, err := rpc.ConvertOperation(opObj, "rpc")
	require.NoError(t, err)

	// Verify operation ID matches expected format
	expectedOpID := model.OperationID(blockNum, int32(trxInBlock), int32(opInTrx))
	assert.Equal(t, expectedOpID, result.ID)
}

// TestConvertBlockEmptyTransactions tests block with no transactions
func TestConvertBlockEmptyTransactions(t *testing.T) {
	blockNum := uint32(1000)
	timestamp := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	protocolBlock := &protocolapi.Block{
		BlockId:      "block1",
		Previous:     "previous1",
		Witness:      "witness1",
		Timestamp:    &protocol.Time{Time: &timestamp},
		Transactions: []protocolapi.Transaction{}, // Empty transactions
	}

	result, err := rpc.ConvertBlock(protocolBlock, blockNum)
	require.NoError(t, err)
	assert.Equal(t, 0, result.TransactionCount)
}

// TestConvertOperationOpType tests operation type conversion
func TestConvertOperationOpType(t *testing.T) {
	testCases := []struct {
		name         string
		operation    protocol.Operation
		expectedType string
	}{
		{
			name: "transfer",
			operation: &protocol.TransferOperation{
				From:   "alice",
				To:     "bob",
				Amount: "1.000 STEEM",
				Memo:   "",
			},
			expectedType: "transfer",
		},
		{
			name: "comment_reward",
			operation: &protocol.CommentRewardOperation{
				Author:   "alice",
				Permlink: "test-post",
				Payout:   "1.000 STEEM",
			},
			expectedType: "comment_reward",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opObj := &protocol.OperationObject{
				BlockNumber:            1000,
				TransactionInBlock:     0,
				OperationInTransaction: 0,
				TransactionID:          "tx1",
				VirtualOperation:       0,
				Operation:              tc.operation,
			}

			result, err := rpc.ConvertOperation(opObj, "rpc")
			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, result.OpType)
		})
	}
}
