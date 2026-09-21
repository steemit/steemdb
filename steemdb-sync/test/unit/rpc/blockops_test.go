package rpc_test

import (
	"testing"

	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/rpc"
	"github.com/steemit/steemutil/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func regularOp(blockNum uint32, trx uint32, memo string) *protocol.OperationObject {
	return &protocol.OperationObject{
		BlockNumber:        blockNum,
		TransactionID:      "tx-regular",
		TransactionInBlock: trx,
		Operation: &protocol.TransferOperation{
			From:   "alice",
			To:     "bob",
			Amount: "1.000 STEEM",
			Memo:   memo,
		},
		VirtualOperation: 0,
	}
}

func virtualOp(blockNum uint32, trx uint32, opInTrx uint16) *protocol.OperationObject {
	return &protocol.OperationObject{
		BlockNumber:            blockNum,
		TransactionInBlock:     trx,
		OperationInTransaction: opInTrx,
		Operation: &protocol.CommentRewardOperation{
			Author:   "alice",
			Permlink: "test-post",
			Payout:   "1.000 STEEM",
		},
		VirtualOperation: 1,
	}
}

// TestConvertBlockOpsMultiOpTransactionRenumbers pins the condenser quirk
// that makes shared renumbering mandatory: get_ops_in_block reports
// op_in_trx=0 for EVERY op of a multi-op transaction, so the raw coordinates
// of a 2-op transaction collapse onto one _id. ConvertBlockOps must assign
// distinct, position-based ids instead.
func TestConvertBlockOpsMultiOpTransactionRenumbers(t *testing.T) {
	blockNum := uint32(1000)
	// Two ops of transaction 0, both reported with op_in_trx=0 (the quirk).
	all := []*protocol.OperationObject{
		regularOp(blockNum, 0, "first"),
		regularOp(blockNum, 0, "second"),
	}

	ops := rpc.ConvertBlockOps(blockNum, all, nil)

	require.Len(t, ops, 2)
	assert.Equal(t, model.OperationID(blockNum, 0, 0), ops[0].ID)
	assert.Equal(t, model.OperationID(blockNum, 0, 1), ops[1].ID)
	assert.NotEqual(t, ops[0].ID, ops[1].ID, "ops of a multi-op transaction must not share one _id")
	assert.Equal(t, int32(0), ops[0].OpIndex)
	assert.Equal(t, int32(1), ops[1].OpIndex)
	assert.False(t, ops[0].Virtual)
	assert.False(t, ops[1].Virtual)
}

// TestConvertBlockOpsPerTransactionNumbering checks that renumbering restarts
// at 0 for each transaction and keeps the RPC-reported trx_in_block.
func TestConvertBlockOpsPerTransactionNumbering(t *testing.T) {
	blockNum := uint32(1000)
	all := []*protocol.OperationObject{
		regularOp(blockNum, 0, "a"),
		regularOp(blockNum, 0, "b"),
		regularOp(blockNum, 1, "c"),
	}

	ops := rpc.ConvertBlockOps(blockNum, all, nil)

	require.Len(t, ops, 3)
	assert.Equal(t, model.OperationID(blockNum, 0, 0), ops[0].ID)
	assert.Equal(t, model.OperationID(blockNum, 0, 1), ops[1].ID)
	assert.Equal(t, model.OperationID(blockNum, 1, 0), ops[2].ID)
}

// TestConvertBlockOpsSkipsVirtualEntriesInAllOps checks that virtual ops
// interleaved in the all-ops listing (reported at the triggering
// transaction's coordinates) are skipped there and only taken from the
// dedicated virtual list.
func TestConvertBlockOpsSkipsVirtualEntriesInAllOps(t *testing.T) {
	blockNum := uint32(1000)
	all := []*protocol.OperationObject{
		regularOp(blockNum, 0, "a"),
		virtualOp(blockNum, 0, 0), // virtual op at the regular op's coordinates
		regularOp(blockNum, 0, "b"),
	}
	virtual := []*protocol.OperationObject{
		virtualOp(blockNum, 0, 0),
	}

	ops := rpc.ConvertBlockOps(blockNum, all, virtual)

	// Two regular ops (renumbered despite the interleaved virtual entry) +
	// one virtual op from the dedicated list.
	require.Len(t, ops, 3)
	assert.Equal(t, model.OperationID(blockNum, 0, 0), ops[0].ID)
	assert.Equal(t, model.OperationID(blockNum, 0, 1), ops[1].ID)
	assert.Equal(t, model.OperationID(blockNum, -1, 1), ops[2].ID)
	assert.True(t, ops[2].Virtual)
}

// TestConvertBlockOpsVirtualCoordinatesNormalized pins the plugin's virtual
// op convention: whatever unusable coordinates the RPC reports (triggering
// transaction coordinates or 0xFFFFFFFF), the produced ids are
// trx_index=-1 (0xFFFFFFFF int32-cast), op_index=1..n.
func TestConvertBlockOpsVirtualCoordinatesNormalized(t *testing.T) {
	blockNum := uint32(1000)
	virtual := []*protocol.OperationObject{
		virtualOp(blockNum, 3, 7), // junk coordinates from the RPC
		virtualOp(blockNum, 0xFFFFFFFF, 0),
	}

	ops := rpc.ConvertBlockOps(blockNum, nil, virtual)

	require.Len(t, ops, 2)
	for i, op := range ops {
		assert.Equal(t, model.OperationID(blockNum, -1, int32(i+1)), op.ID)
		assert.Equal(t, int32(-1), op.TrxIndex)
		assert.Equal(t, int32(i+1), op.OpIndex)
		assert.True(t, op.Virtual)
	}
}

// TestConvertBlockOpsPluginCoordinateParity pins the full id set for a
// representative mixed block so replay-sourced (plugin) and RPC-sourced ids
// stay interchangeable: regular ops at (trx, 0..n-1), virtual ops at
// (-1, 1..n).
func TestConvertBlockOpsPluginCoordinateParity(t *testing.T) {
	blockNum := uint32(1000)
	all := []*protocol.OperationObject{
		regularOp(blockNum, 0, "a"),
		regularOp(blockNum, 0, "b"),
		regularOp(blockNum, 2, "c"),
	}
	virtual := []*protocol.OperationObject{
		virtualOp(blockNum, 2, 0),
		virtualOp(blockNum, 0xFFFFFFFF, 5),
	}

	ops := rpc.ConvertBlockOps(blockNum, all, virtual)

	expected := []string{
		model.OperationID(blockNum, 0, 0),
		model.OperationID(blockNum, 0, 1),
		model.OperationID(blockNum, 2, 0),
		model.OperationID(blockNum, -1, 1),
		model.OperationID(blockNum, -1, 2),
	}
	require.Len(t, ops, len(expected))
	for i, id := range expected {
		assert.Equal(t, id, ops[i].ID)
	}

	// All ids pairwise distinct (no upsert collapse).
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		assert.False(t, seen[op.ID], "duplicate _id %s", op.ID)
		seen[op.ID] = true
	}
}
