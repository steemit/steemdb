package utils

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// BlocksProcessed tracks the total number of blocks processed
	BlocksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_blocks_processed_total",
			Help: "The total number of blocks processed",
		},
		[]string{"service"},
	)

	// OperationsProcessed tracks the total number of operations processed
	OperationsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_operations_processed_total",
			Help: "The total number of operations processed",
		},
		[]string{"operation_type"},
	)

	// ProcessingDuration tracks the time spent processing blocks/operations
	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "steemdb_processing_duration_seconds",
			Help:    "Time spent processing blocks and operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "operation"},
	)

	// ErrorsTotal tracks the total number of errors
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_errors_total",
			Help: "The total number of errors encountered",
		},
		[]string{"service", "error_type"},
	)

	// DatabaseOperations tracks database operations
	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_database_operations_total",
			Help: "The total number of database operations",
		},
		[]string{"operation", "collection"},
	)

	// RPCRequests tracks RPC requests to Steem nodes
	RPCRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_rpc_requests_total",
			Help: "The total number of RPC requests to Steem nodes",
		},
		[]string{"method", "node", "status"},
	)

	// CurrentBlock tracks the current block being processed
	CurrentBlock = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "steemdb_current_block",
			Help: "The current block number being processed",
		},
		[]string{"service"},
	)

	// QueueSize tracks the size of processing queues
	QueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "steemdb_queue_size",
			Help: "The current size of processing queues",
		},
		[]string{"queue_type"},
	)

	// ActiveWorkers tracks the number of active workers
	ActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "steemdb_active_workers",
			Help: "The number of active worker goroutines",
		},
		[]string{"service"},
	)

	// MemoryUsage tracks memory usage
	MemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "steemdb_memory_usage_bytes",
			Help: "Memory usage in bytes",
		},
		[]string{"type"},
	)
)
