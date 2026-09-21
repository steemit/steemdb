package rpc

import (
	"log"

	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemutil/protocol"
)

// ConvertBlockOps converts one block's operations into model operations with
// the SAME operation ids the ingest plugin produces, so replay-sourced and
// RPC-sourced blocks are interchangeable under idempotent upserts.
//
// This is the single implementation of the RPC op-coordinate conventions,
// shared by every RPC-based ingest path (live_sync, repair) — do not
// re-derive the conventions per binary; ids that drift from the plugin's
// numbering collide or duplicate under upserts.
//
// Two quirks of condenser_api.get_ops_in_block make the raw fields unusable:
//   - in the all-ops list, op_in_trx is reported as 0 for EVERY operation of
//     a multi-op transaction (a 17-vote transaction yields seventeen
//     (trx, 0) pairs), so the position within a transaction has to be
//     reconstructed from array order;
//   - virtual ops (virtual_op > 0) appear in the all-ops list at either the
//     triggering transaction's coordinates or 0xFFFFFFFF, neither of which
//     matches the plugin, which numbers them trx_index=-1 with op_index
//     starting at 1.
func ConvertBlockOps(blockNum uint32, all, virtual []*protocol.OperationObject) []*model.Operation {
	ops := make([]*model.Operation, 0, len(all)+len(virtual))
	appendOp := func(opObj *protocol.OperationObject, what string) {
		modelOp, err := ConvertOperation(opObj, "rpc")
		if err != nil {
			// Never silent: a dropped op is a permanent hole in the window.
			log.Printf("[BlockOps] Drop %s op in block %d (trx=%d op=%d): %v",
				what, blockNum, opObj.TransactionInBlock, opObj.OperationInTransaction, err)
			return
		}
		ops = append(ops, modelOp)
	}

	// Regular ops from the all-ops list, numbered by array position within
	// each transaction.
	perTrx := make(map[uint32]int)
	for _, opObj := range all {
		if opObj.VirtualOperation > 0 {
			continue
		}
		idx := perTrx[opObj.TransactionInBlock]
		perTrx[opObj.TransactionInBlock] = idx + 1
		opObj.OperationInTransaction = uint16(idx)
		appendOp(opObj, "regular")
	}

	// Virtual ops from the dedicated list: plugin numbering is trx=-1
	// (0xFFFFFFFF, which int32-casts to -1) with op_index 1,2,3...
	for i, opObj := range virtual {
		opObj.TransactionInBlock = 0xFFFFFFFF
		opObj.OperationInTransaction = uint16(i + 1)
		appendOp(opObj, "virtual")
	}
	return ops
}
