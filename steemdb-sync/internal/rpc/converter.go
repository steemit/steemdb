package rpc

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	protocolapi "github.com/steemit/steemutil/protocol/api"
	"github.com/steemit/steemutil/protocol"
	"github.com/steemit/steemdb-sync/internal/model"
)

// ConvertBlock converts protocolapi.Block to model.Block
func ConvertBlock(block *protocolapi.Block, blockNum uint32) (*model.Block, error) {
	if block == nil {
		return nil, errors.New("block is nil")
	}

	// Get timestamp from protocol.Time
	var timestamp time.Time
	if block.Timestamp != nil && block.Timestamp.Time != nil {
		timestamp = *block.Timestamp.Time
	} else {
		timestamp = time.Now()
	}

	return &model.Block{
		ID:               blockNum,
		BlockNum:         blockNum,
		BlockID:          block.BlockId,
		Previous:         block.Previous,
		Timestamp:        timestamp,
		Witness:          block.Witness,
		TransactionCount: len(block.Transactions),
	}, nil
}

// ConvertTransaction converts protocol transaction to model.Transaction
func ConvertTransaction(trx *protocolapi.Transaction, blockNum uint32, trxIndex int32) (*model.Transaction, error) {
	if trx == nil {
		return nil, errors.New("transaction is nil")
	}

	// Get expiration from protocol.Time
	var expiration time.Time
	if trx.Expiration != nil && trx.Expiration.Time != nil {
		expiration = *trx.Expiration.Time
	} else {
		expiration = time.Now()
	}

	return &model.Transaction{
		ID:         trx.TransactionId,
		BlockNum:   blockNum,
		TrxIndex:   trxIndex,
		Expiration: expiration,
	}, nil
}

// ConvertOperation converts protocol.OperationObject to model.Operation
func ConvertOperation(opObj *protocol.OperationObject, source string) (*model.Operation, error) {
	if opObj == nil {
		return nil, errors.New("operation object is nil")
	}

	// Determine if virtual
	isVirtual := opObj.VirtualOperation > 0

	// Generate operation ID
	trxIndex := int32(opObj.TransactionInBlock)
	opIndex := int32(opObj.OperationInTransaction)
	opID := model.OperationID(opObj.BlockNumber, trxIndex, opIndex)

	// Get operation type (OpType is already a string)
	opType := string(opObj.Operation.Type())

	// Convert operation data to map
	opValue, err := operationDataToMap(opObj.Operation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert operation data")
	}

	return &model.Operation{
		ID:       opID,
		BlockNum: opObj.BlockNumber,
		TrxID:    opObj.TransactionID,
		TrxIndex: trxIndex,
		OpIndex:  opIndex,
		OpType:   opType,
		OpValue:  opValue,
		Virtual:  isVirtual,
		Source:   source,
	}, nil
}

// operationDataToMap converts protocol.Operation to map[string]interface{}
func operationDataToMap(op protocol.Operation) (map[string]interface{}, error) {
	// Use JSON marshaling/unmarshaling to convert
	data := op.Data()
	
	// If data is already a map, return it
	if m, ok := data.(map[string]interface{}); ok {
		return m, nil
	}

	// Otherwise, marshal to JSON and unmarshal to map
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal operation data")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal operation data")
	}

	return result, nil
}
