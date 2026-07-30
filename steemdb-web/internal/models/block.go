package models

import (
	"time"
)

// Block represents a blockchain block.
// Aligned with sync's model.Block: _id is the block number (uint32),
// block_num mirrors _id, timestamp is the block time.
type Block struct {
	ID                    uint32                   `json:"id" bson:"_id"`
	BlockNum              uint32                   `json:"block_num" bson:"block_num"`
	BlockID               string                   `json:"block_id" bson:"block_id"`
	Previous              string                   `json:"previous" bson:"previous"`
	Timestamp             time.Time                `json:"timestamp" bson:"timestamp"`
	Witness               string                   `json:"witness" bson:"witness"`
	TransactionMerkleRoot string                   `json:"transaction_merkle_root" bson:"transaction_merkle_root"`
	Extensions            []interface{}            `json:"extensions" bson:"extensions"`
	WitnessSignature      string                   `json:"witness_signature" bson:"witness_signature"`
	Transactions          []Transaction            `json:"transactions" bson:"transactions"`
	TransactionCount      int                      `json:"transaction_count" bson:"transaction_count"`
	OperationCount        int                      `json:"operation_count" bson:"operation_count"`
	Created               time.Time                `json:"created" bson:"created"`
}

// Transaction represents a blockchain transaction
type Transaction struct {
	ID                string      `json:"id" bson:"_id"`
	RefBlockNum       int         `json:"ref_block_num" bson:"ref_block_num"`
	RefBlockPrefix    int64       `json:"ref_block_prefix" bson:"ref_block_prefix"`
	Expiration        time.Time   `json:"expiration" bson:"expiration"`
	Operations        []Operation `json:"operations" bson:"operations"`
	Extensions        []interface{} `json:"extensions" bson:"extensions"`
	Signatures        []string    `json:"signatures" bson:"signatures"`
	TransactionID     string      `json:"transaction_id" bson:"transaction_id"`
	BlockNum          int64       `json:"block_num" bson:"block_num"`
	TransactionNum    int         `json:"transaction_num" bson:"transaction_num"`
	Timestamp         time.Time   `json:"timestamp" bson:"timestamp"`
}

// Operation represents a blockchain operation.
// Aligned with sync's model.Operation: _id is "block:trx:op" string,
// op_type/op_value/trx_index/op_index match sync field names.
type Operation struct {
	ID        string                 `json:"id" bson:"_id"`
	BlockNum  uint32                 `json:"block_num" bson:"block_num"`
	TrxID     string                 `json:"trx_id" bson:"trx_id"`
	TrxIndex  int32                  `json:"trx_index" bson:"trx_index"`
	OpIndex   int32                  `json:"op_index" bson:"op_index"`
	OpType    string                 `json:"op_type" bson:"op_type"`
	OpValue   map[string]interface{} `json:"op_value" bson:"op_value"`
	Virtual   bool                   `json:"virtual" bson:"virtual"`
	Source    string                 `json:"source" bson:"source"`
}

// BlockSummary represents a simplified block view for lists
type BlockSummary struct {
	Number           uint32    `json:"number"`        // mirrors block_num / _id
	Timestamp        time.Time `json:"timestamp"`
	Witness          string    `json:"witness"`
	TransactionCount int       `json:"transaction_count"`
	OperationCount   int       `json:"operation_count"`
	Previous         string    `json:"previous"`
}

// BlockStats represents blockchain statistics
type BlockStats struct {
	LatestBlockNum       int64     `json:"latest_block_num"`
	LastIrreversibleNum  int64     `json:"last_irreversible_num"`
	HeadBlockTime        time.Time `json:"head_block_time"`
	TotalTransactions    int64     `json:"total_transactions"`
	TotalOperations      int64     `json:"total_operations"`
	AverageBlockTime     float64   `json:"average_block_time"`
	TransactionsPerHour  int64     `json:"transactions_per_hour"`
	OperationsPerHour    int64     `json:"operations_per_hour"`
}

// OperationStats represents operation statistics
type OperationStats struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
	Percentage float64 `json:"percentage"`
}

// BlockSearchResult represents block search results
type BlockSearchResult struct {
	Blocks   []BlockSummary `json:"blocks"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
