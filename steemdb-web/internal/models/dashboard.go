package models

// NetworkPerformance represents network performance metrics
type NetworkPerformance struct {
	Transactions24h       int64   `json:"transactions_24h"`
	Transactions1h        int64   `json:"transactions_1h"`
	TransactionsPerSec24h float64 `json:"transactions_per_sec_24h"`
	TransactionsPerSec1h  float64 `json:"transactions_per_sec_1h"`
	Operations24h         int64   `json:"operations_24h"`
	Operations1h          int64   `json:"operations_1h"`
	OperationsPerSec24h   float64 `json:"operations_per_sec_24h"`
	OperationsPerSec1h    float64 `json:"operations_per_sec_1h"`
}
