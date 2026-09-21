package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/steemit/steemutil/protocol"
	protocolapi "github.com/steemit/steemutil/protocol/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/steemit/steemdb-sync/internal/model"
)

// fakeFetcher serves canned RPC responses.
type fakeFetcher struct {
	block      *protocolapi.Block
	allOps     []*protocol.OperationObject
	virtualOps []*protocol.OperationObject
}

func (f *fakeFetcher) GetBlock(ctx context.Context, blockNum uint32) (*protocolapi.Block, error) {
	return f.block, nil
}

func (f *fakeFetcher) GetOpsInBlock(ctx context.Context, blockNum uint32, onlyVirtual bool) ([]*protocol.OperationObject, error) {
	if onlyVirtual {
		return f.virtualOps, nil
	}
	return f.allOps, nil
}

// recordingWriter records the order of the write calls.
type recordingWriter struct {
	calls    []string
	ops      []*model.Operation
	txs      []*model.Transaction
	blocks   []*model.Block
	maxBlock uint32
}

func (w *recordingWriter) BulkUpsertOperations(ctx context.Context, ops []*model.Operation) error {
	w.calls = append(w.calls, "operations")
	w.ops = append(w.ops, ops...)
	return nil
}

func (w *recordingWriter) BulkUpsertTransactions(ctx context.Context, txs []*model.Transaction) error {
	w.calls = append(w.calls, "transactions")
	w.txs = append(w.txs, txs...)
	return nil
}

func (w *recordingWriter) BulkUpsertBlocks(ctx context.Context, blocks []*model.Block) error {
	w.calls = append(w.calls, "blocks")
	w.blocks = append(w.blocks, blocks...)
	return nil
}

func (w *recordingWriter) GetMaxBlock(ctx context.Context) (uint32, error) {
	return w.maxBlock, nil
}

func (w *recordingWriter) UpdateMaxBlock(ctx context.Context, blockNum uint32) error {
	w.calls = append(w.calls, "max_block")
	w.maxBlock = blockNum
	return nil
}

func testBlock() *protocolapi.Block {
	return testBlockWithTxs(2)
}

func testBlockWithTxs(n int) *protocolapi.Block {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	exp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	txs := make([]protocolapi.Transaction, 0, n)
	for i := 0; i < n; i++ {
		txs = append(txs, protocolapi.Transaction{
			TransactionId: fmt.Sprintf("tx%d", i),
			Expiration:    &protocol.Time{Time: &exp},
		})
	}
	return &protocolapi.Block{
		BlockId:      "blockid",
		Previous:     "previous",
		Witness:      "witness1",
		Timestamp:    &protocol.Time{Time: &ts},
		Transactions: txs,
	}
}

func transferOp(blockNum, trx uint32, memo string) *protocol.OperationObject {
	return &protocol.OperationObject{
		BlockNumber:        blockNum,
		TransactionID:      "tx0",
		TransactionInBlock: trx,
		Operation: &protocol.TransferOperation{
			From:   "alice",
			To:     "bob",
			Amount: "1.000 STEEM",
			Memo:   memo,
		},
	}
}

func virtualOp(blockNum, trx uint32, opInTrx uint16) *protocol.OperationObject {
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

// TestRepairBlockWriteOrder asserts the crash-consistency contract shared
// with cmd/live_sync: operations → transactions → block header → max_block.
// A block header must never land before its operations, or a partial ops
// failure becomes invisible to the scanner and to live_sync's resume logic.
func TestRepairBlockWriteOrder(t *testing.T) {
	blockNum := uint32(1000)
	fetcher := &fakeFetcher{
		block: testBlock(),
		allOps: []*protocol.OperationObject{
			transferOp(blockNum, 0, "a"),
			transferOp(blockNum, 0, "b"),
		},
	}
	writer := &recordingWriter{maxBlock: 5000}

	txCount, opCount, err := repairBlock(context.Background(), fetcher, writer, blockNum)
	require.NoError(t, err)
	assert.Equal(t, 2, txCount)
	assert.Equal(t, 2, opCount)

	assert.Equal(t, []string{"operations", "transactions", "blocks"}, writer.calls[:3],
		"write order must be ops → transactions → blocks")
	// max_block only moves forward; block 1000 is below the stored 5000, so
	// no update is issued.
	assert.Equal(t, []string{"operations", "transactions", "blocks"}, writer.calls)
}

// TestRepairBlockWritesMaxBlockLast asserts the max_block commit happens
// after every document write when the repaired block extends the height.
func TestRepairBlockWritesMaxBlockLast(t *testing.T) {
	blockNum := uint32(6000)
	fetcher := &fakeFetcher{
		block:  testBlock(),
		allOps: []*protocol.OperationObject{transferOp(blockNum, 0, "a")},
	}
	writer := &recordingWriter{maxBlock: 5000}

	_, _, err := repairBlock(context.Background(), fetcher, writer, blockNum)
	require.NoError(t, err)

	assert.Equal(t, []string{"operations", "transactions", "blocks", "max_block"}, writer.calls)
	assert.Equal(t, blockNum, writer.maxBlock)
}

// TestRepairBlockMultiOpTransactionDistinctIDs guards the F3 fix: the
// condenser API reports op_in_trx=0 for every op of a multi-op transaction;
// repair must write each op with the renumbered (plugin-convention) id
// instead of collapsing them onto one _id that overwrites itself.
func TestRepairBlockMultiOpTransactionDistinctIDs(t *testing.T) {
	blockNum := uint32(1000)
	fetcher := &fakeFetcher{
		block: testBlock(),
		// Raw RPC shape: both ops of transaction 0 report op_in_trx=0.
		allOps: []*protocol.OperationObject{
			transferOp(blockNum, 0, "first"),
			transferOp(blockNum, 0, "second"),
		},
	}
	writer := &recordingWriter{maxBlock: 5000}

	_, opCount, err := repairBlock(context.Background(), fetcher, writer, blockNum)
	require.NoError(t, err)

	require.Equal(t, 2, opCount)
	require.Len(t, writer.ops, 2)
	assert.Equal(t, model.OperationID(blockNum, 0, 0), writer.ops[0].ID)
	assert.Equal(t, model.OperationID(blockNum, 0, 1), writer.ops[1].ID)
	assert.NotEqual(t, writer.ops[0].ID, writer.ops[1].ID)
}

// TestRepairBlockVirtualOpCoordinates asserts repair normalizes virtual op
// coordinates to the plugin convention (trx_index=-1, op_index=1..n),
// keeping ids interchangeable with live_sync/plugin-sourced documents.
func TestRepairBlockVirtualOpCoordinates(t *testing.T) {
	blockNum := uint32(1000)
	fetcher := &fakeFetcher{
		block: testBlock(),
		// Junk virtual coordinates as reported by the RPC.
		virtualOps: []*protocol.OperationObject{
			virtualOp(blockNum, 0, 0),
			virtualOp(blockNum, 3, 7),
		},
	}
	writer := &recordingWriter{maxBlock: 5000}

	_, opCount, err := repairBlock(context.Background(), fetcher, writer, blockNum)
	require.NoError(t, err)

	require.Equal(t, 2, opCount)
	require.Len(t, writer.ops, 2)
	assert.Equal(t, model.OperationID(blockNum, -1, 1), writer.ops[0].ID)
	assert.Equal(t, model.OperationID(blockNum, -1, 2), writer.ops[1].ID)
	assert.Equal(t, int32(-1), writer.ops[0].TrxIndex)
	assert.True(t, writer.ops[0].Virtual)
}

// TestRepairBlockEmptyBlock checks the empty-block path: no ops/transactions
// writes, header still lands (and marks the block present).
func TestRepairBlockEmptyBlock(t *testing.T) {
	fetcher := &fakeFetcher{block: testBlockWithTxs(0)}
	writer := &recordingWriter{maxBlock: 5000}

	txCount, opCount, err := repairBlock(context.Background(), fetcher, writer, 1000)
	require.NoError(t, err)
	assert.Equal(t, 0, txCount)
	assert.Equal(t, 0, opCount)

	assert.Equal(t, []string{"blocks"}, writer.calls)
	require.Len(t, writer.blocks, 1)
	assert.Equal(t, uint32(1000), writer.blocks[0].BlockNum)
}
