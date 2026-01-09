package model

import (
	"fmt"
	"time"
)

// Block represents a Steem block
type Block struct {
	ID               uint32    `bson:"_id" json:"id"`
	BlockNum         uint32    `bson:"block_num" json:"block_num"`
	BlockID          string    `bson:"block_id" json:"block_id"`
	Previous         string    `bson:"previous" json:"previous"`
	Timestamp        time.Time `bson:"timestamp" json:"timestamp"`
	Witness          string    `bson:"witness" json:"witness"`
	TransactionCount int       `bson:"transaction_count" json:"transaction_count"`
}

// Transaction represents a Steem transaction
type Transaction struct {
	ID          string    `bson:"_id" json:"id"`
	BlockNum    uint32    `bson:"block_num" json:"block_num"`
	TrxIndex    int32     `bson:"trx_index" json:"trx_index"`
	Expiration  time.Time `bson:"expiration" json:"expiration"`
}

// Operation represents a Steem operation (real or virtual)
type Operation struct {
	ID        string                 `bson:"_id" json:"id"`
	BlockNum  uint32                 `bson:"block_num" json:"block_num"`
	TrxID     string                 `bson:"trx_id" json:"trx_id"`
	TrxIndex  int32                  `bson:"trx_index" json:"trx_index"`
	OpIndex   int32                  `bson:"op_index" json:"op_index"`
	OpType    string                 `bson:"op_type" json:"op_type"`
	OpValue   map[string]interface{} `bson:"op_value" json:"op_value"`
	Virtual   bool                   `bson:"virtual" json:"virtual"`
	Source    string                 `bson:"source" json:"source"` // "plugin" or "rpc"
}

// Meta represents synchronization metadata
type Meta struct {
	ID            string    `bson:"_id" json:"id"`
	MaxBlock      uint32    `bson:"max_block" json:"max_block"`
	ColdStartDone bool      `bson:"cold_start_done" json:"cold_start_done"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
}

// OperationID generates a unique ID for an operation
func OperationID(blockNum uint32, trxIndex, opIndex int32) string {
	return fmt.Sprintf("%d:%d:%d", blockNum, trxIndex, opIndex)
}
