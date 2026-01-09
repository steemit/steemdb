package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Ingest metrics
	IngestOpsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_sync_ingest_ops_total",
			Help: "Total number of operations ingested",
		},
		[]string{"source"}, // source: "plugin" or "rpc"
	)

	IngestOpsTPS = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "steemdb_sync_ingest_ops_per_second",
			Help: "Operations ingested per second",
		},
	)

	// MongoDB write metrics
	MongoWriteDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "steemdb_sync_mongo_write_duration_seconds",
			Help:    "MongoDB write operation duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
		[]string{"collection", "operation"}, // collection: "blocks", "transactions", "operations"
	)

	MongoWriteTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_sync_mongo_write_total",
			Help: "Total number of MongoDB write operations",
		},
		[]string{"collection", "operation", "status"}, // status: "success" or "error"
	)

	// RPC metrics
	RPCLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "steemdb_sync_rpc_latency_seconds",
			Help:    "RPC request latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
		[]string{"method"}, // method: "get_block", "get_ops_in_block"
	)

	RPCTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "steemdb_sync_rpc_total",
			Help: "Total number of RPC requests",
		},
		[]string{"method", "status"}, // status: "success" or "error"
	)

	// Batch metrics
	BatchSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "steemdb_sync_batch_size",
			Help:    "Batch size distribution",
			Buckets: prometheus.LinearBuckets(10, 100, 20), // 10 to 2000
		},
	)

	BatchFlushDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "steemdb_sync_batch_flush_duration_seconds",
			Help:    "Batch flush duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
	)

	// Queue metrics
	QueueSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "steemdb_sync_queue_size",
			Help: "Current queue size",
		},
	)

	// Block metrics
	CurrentBlock = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "steemdb_sync_current_block",
			Help: "Current block number being processed",
		},
	)
)

func init() {
	// Register all metrics
	prometheus.MustRegister(IngestOpsTotal)
	prometheus.MustRegister(IngestOpsTPS)
	prometheus.MustRegister(MongoWriteDuration)
	prometheus.MustRegister(MongoWriteTotal)
	prometheus.MustRegister(RPCLatency)
	prometheus.MustRegister(RPCTotal)
	prometheus.MustRegister(BatchSize)
	prometheus.MustRegister(BatchFlushDuration)
	prometheus.MustRegister(QueueSize)
	prometheus.MustRegister(CurrentBlock)
}

// RecordIngestOp records an ingested operation
func RecordIngestOp(source string) {
	IngestOpsTotal.WithLabelValues(source).Inc()
	IncrementOpCount()
}

// RecordMongoWrite records a MongoDB write operation
func RecordMongoWrite(collection, operation string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	MongoWriteDuration.WithLabelValues(collection, operation).Observe(duration.Seconds())
	MongoWriteTotal.WithLabelValues(collection, operation, status).Inc()
}

// RecordRPCCall records an RPC call
func RecordRPCCall(method string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	RPCLatency.WithLabelValues(method).Observe(duration.Seconds())
	RPCTotal.WithLabelValues(method, status).Inc()
}

// RecordBatch records a batch operation
func RecordBatch(size int, duration time.Duration) {
	BatchSize.Observe(float64(size))
	BatchFlushDuration.Observe(duration.Seconds())
}

// UpdateQueueSize updates the queue size gauge
func UpdateQueueSize(size int) {
	QueueSize.Set(float64(size))
}

// UpdateCurrentBlock updates the current block gauge
func UpdateCurrentBlock(blockNum uint32) {
	CurrentBlock.Set(float64(blockNum))
}

// UpdateIngestTPS updates the ingest TPS gauge
func UpdateIngestTPS(tps float64) {
	IngestOpsTPS.Set(tps)
}

// Handler returns the HTTP handler for Prometheus metrics
func Handler() http.Handler {
	return promhttp.Handler()
}

// TPSCalculator calculates TPS from operation count
type TPSCalculator struct {
	opCount   int64
	lastCount int64
	lastTime  time.Time
	mu        sync.RWMutex
}

var tpsCalculator = &TPSCalculator{
	lastTime: time.Now(),
}

// IncrementOpCount increments the operation count (thread-safe)
func IncrementOpCount() {
	tpsCalculator.mu.Lock()
	defer tpsCalculator.mu.Unlock()
	tpsCalculator.opCount++
}

// GetOpCount returns the current operation count
func GetOpCount() int64 {
	tpsCalculator.mu.RLock()
	defer tpsCalculator.mu.RUnlock()
	return tpsCalculator.opCount
}

// StartTPSCalculator starts a goroutine that calculates and updates TPS periodically
func StartTPSCalculator(updateInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()

		tpsCalculator.mu.Lock()
		lastCount := tpsCalculator.opCount
		lastTime := tpsCalculator.lastTime
		tpsCalculator.mu.Unlock()

		for range ticker.C {
			tpsCalculator.mu.Lock()
			currentCount := tpsCalculator.opCount
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()
			
			if elapsed > 0 {
				tps := float64(currentCount-lastCount) / elapsed
				UpdateIngestTPS(tps)
			}

			lastCount = currentCount
			lastTime = now
			tpsCalculator.mu.Unlock()
		}
	}()
}
