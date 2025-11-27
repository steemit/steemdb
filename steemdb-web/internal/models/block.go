package models

import (
	"time"
)

// Block represents a blockchain block
type Block struct {
	ID                    string                   `json:"id" bson:"_id"`
	Number                int64                    `json:"number" bson:"number"`
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

// Operation represents a blockchain operation
type Operation struct {
	ID        string                 `json:"id" bson:"_id"`
	Type      string                 `json:"type" bson:"type"`
	Value     map[string]interface{} `json:"value" bson:"value"`
	BlockNum  int64                  `json:"block_num" bson:"block_num"`
	TxNum     int                    `json:"tx_num" bson:"tx_num"`
	OpNum     int                    `json:"op_num" bson:"op_num"`
	Timestamp time.Time              `json:"timestamp" bson:"timestamp"`
	Virtual   bool                   `json:"virtual" bson:"virtual"`
}

// BlockSummary represents a simplified block view for lists
type BlockSummary struct {
	Number           int64     `json:"number"`
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
