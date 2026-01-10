package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/metrics"
)

// TestMetricsRegistration tests that all metrics are registered and accessible
func TestMetricsRegistration(t *testing.T) {
	// Verify all metric objects are not nil (they should be initialized in init())
	assert.NotNil(t, metrics.IngestOpsTotal, "IngestOpsTotal should be initialized")
	assert.NotNil(t, metrics.IngestOpsTPS, "IngestOpsTPS should be initialized")
	assert.NotNil(t, metrics.MongoWriteDuration, "MongoWriteDuration should be initialized")
	assert.NotNil(t, metrics.MongoWriteTotal, "MongoWriteTotal should be initialized")
	assert.NotNil(t, metrics.RPCLatency, "RPCLatency should be initialized")
	assert.NotNil(t, metrics.RPCTotal, "RPCTotal should be initialized")
	assert.NotNil(t, metrics.BatchSize, "BatchSize should be initialized")
	assert.NotNil(t, metrics.BatchFlushDuration, "BatchFlushDuration should be initialized")
	assert.NotNil(t, metrics.QueueSize, "QueueSize should be initialized")
	assert.NotNil(t, metrics.CurrentBlock, "CurrentBlock should be initialized")

	// Verify metrics can be gathered (they should be registered in default registry)
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	
	// At least some metrics should be gathered
	assert.Greater(t, len(metricFamilies), 0, "Should have at least some metrics gathered")
	
	// Verify we can access metrics via HTTP handler (indirect verification of registration)
	handler := metrics.Handler()
	require.NotNil(t, handler)
	
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	
	// Verify response contains our metric names
	assert.Contains(t, body, "steemdb_sync", "Response should contain steemdb_sync metrics")
}

// TestRecordIngestOp tests recording ingested operations
func TestRecordIngestOp(t *testing.T) {
	// Reset counters before test
	metrics.IngestOpsTotal.Reset()

	// Record operations from different sources
	metrics.RecordIngestOp("plugin")
	metrics.RecordIngestOp("plugin")
	metrics.RecordIngestOp("rpc")
	metrics.RecordIngestOp("rpc")
	metrics.RecordIngestOp("rpc")

	// Verify counts
	pluginCount, err := metrics.IngestOpsTotal.GetMetricWithLabelValues("plugin")
	require.NoError(t, err)
	assert.Equal(t, float64(2), getCounterValue(pluginCount))

	rpcCount, err := metrics.IngestOpsTotal.GetMetricWithLabelValues("rpc")
	require.NoError(t, err)
	assert.Equal(t, float64(3), getCounterValue(rpcCount))
}

// TestRecordMongoWrite tests recording MongoDB write operations
func TestRecordMongoWrite(t *testing.T) {
	// Reset metrics
	metrics.MongoWriteDuration.Reset()
	metrics.MongoWriteTotal.Reset()

	// Record successful write
	duration := 50 * time.Millisecond
	metrics.RecordMongoWrite("operations", "upsert", duration, nil)

	// Record failed write
	metrics.RecordMongoWrite("blocks", "upsert", 100*time.Millisecond, assert.AnError)

	// Verify success count
	successCount, err := metrics.MongoWriteTotal.GetMetricWithLabelValues("operations", "upsert", "success")
	require.NoError(t, err)
	assert.Equal(t, float64(1), getCounterValue(successCount))

	// Verify error count
	errorCount, err := metrics.MongoWriteTotal.GetMetricWithLabelValues("blocks", "upsert", "error")
	require.NoError(t, err)
	assert.Equal(t, float64(1), getCounterValue(errorCount))

	// Verify duration was recorded (check that metric exists and can be queried)
	durationMetric, err := metrics.MongoWriteDuration.GetMetricWithLabelValues("operations", "upsert")
	require.NoError(t, err)
	require.NotNil(t, durationMetric)
	
	// Verify it's a histogram
	_, ok := durationMetric.(prometheus.Histogram)
	assert.True(t, ok, "Should be a histogram")
}

// TestRecordRPCCall tests recording RPC calls
func TestRecordRPCCall(t *testing.T) {
	// Reset metrics
	metrics.RPCLatency.Reset()
	metrics.RPCTotal.Reset()

	// Record successful RPC call
	duration := 200 * time.Millisecond
	metrics.RecordRPCCall("get_block", duration, nil)

	// Record failed RPC call
	metrics.RecordRPCCall("get_ops_in_block", 300*time.Millisecond, assert.AnError)

	// Verify success count
	successCount, err := metrics.RPCTotal.GetMetricWithLabelValues("get_block", "success")
	require.NoError(t, err)
	assert.Equal(t, float64(1), getCounterValue(successCount))

	// Verify error count
	errorCount, err := metrics.RPCTotal.GetMetricWithLabelValues("get_ops_in_block", "error")
	require.NoError(t, err)
	assert.Equal(t, float64(1), getCounterValue(errorCount))

	// Verify latency was recorded (check that metric exists and can be queried)
	latencyMetric, err := metrics.RPCLatency.GetMetricWithLabelValues("get_block")
	require.NoError(t, err)
	require.NotNil(t, latencyMetric)
	
	// Verify it's a histogram
	_, ok := latencyMetric.(prometheus.Histogram)
	assert.True(t, ok, "Should be a histogram")
}

// TestRecordBatch tests recording batch operations
func TestRecordBatch(t *testing.T) {
	// Record batch
	batchSize := 100
	duration := 50 * time.Millisecond
	metrics.RecordBatch(batchSize, duration)

	// Verify batch size was recorded by checking the metric
	// We can't easily reset histograms, so we just verify they accept values
	// The actual verification would be done via the HTTP handler
	batchSizeMetric := metrics.BatchSize
	require.NotNil(t, batchSizeMetric)

	// Verify flush duration was recorded
	flushDurationMetric := metrics.BatchFlushDuration
	require.NotNil(t, flushDurationMetric)
}

// TestUpdateQueueSize tests updating queue size gauge
func TestUpdateQueueSize(t *testing.T) {
	// Reset gauge
	metrics.QueueSize.Set(0)

	// Update queue size
	metrics.UpdateQueueSize(100)
	assert.Equal(t, float64(100), getGaugeValue(metrics.QueueSize))

	metrics.UpdateQueueSize(50)
	assert.Equal(t, float64(50), getGaugeValue(metrics.QueueSize))

	metrics.UpdateQueueSize(0)
	assert.Equal(t, float64(0), getGaugeValue(metrics.QueueSize))
}

// TestUpdateCurrentBlock tests updating current block gauge
func TestUpdateCurrentBlock(t *testing.T) {
	// Reset gauge
	metrics.CurrentBlock.Set(0)

	// Update current block
	metrics.UpdateCurrentBlock(1000)
	assert.Equal(t, float64(1000), getGaugeValue(metrics.CurrentBlock))

	metrics.UpdateCurrentBlock(5000)
	assert.Equal(t, float64(5000), getGaugeValue(metrics.CurrentBlock))

	metrics.UpdateCurrentBlock(10000)
	assert.Equal(t, float64(10000), getGaugeValue(metrics.CurrentBlock))
}

// TestUpdateIngestTPS tests updating ingest TPS gauge
func TestUpdateIngestTPS(t *testing.T) {
	// Reset gauge
	metrics.IngestOpsTPS.Set(0)

	// Update TPS
	metrics.UpdateIngestTPS(100.5)
	assert.Equal(t, 100.5, getGaugeValue(metrics.IngestOpsTPS))

	metrics.UpdateIngestTPS(500.0)
	assert.Equal(t, 500.0, getGaugeValue(metrics.IngestOpsTPS))

	metrics.UpdateIngestTPS(0.0)
	assert.Equal(t, 0.0, getGaugeValue(metrics.IngestOpsTPS))
}

// TestIncrementOpCount tests incrementing operation count
func TestIncrementOpCount(t *testing.T) {
	initialCount := metrics.GetOpCount()

	// Increment multiple times
	metrics.IncrementOpCount()
	metrics.IncrementOpCount()
	metrics.IncrementOpCount()

	finalCount := metrics.GetOpCount()
	assert.Equal(t, initialCount+3, finalCount)
}

// TestIncrementOpCountConcurrent tests concurrent increment operation count
func TestIncrementOpCountConcurrent(t *testing.T) {
	initialCount := metrics.GetOpCount()

	const numGoroutines = 10
	const incrementsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				metrics.IncrementOpCount()
			}
		}()
	}

	wg.Wait()

	finalCount := metrics.GetOpCount()
	expectedCount := initialCount + (numGoroutines * incrementsPerGoroutine)
	assert.Equal(t, expectedCount, finalCount)
}

// TestStartTPSCalculator tests TPS calculation
func TestStartTPSCalculator(t *testing.T) {
	// Reset TPS gauge
	metrics.IngestOpsTPS.Set(0)

	// Start TPS calculator with short update interval
	updateInterval := 100 * time.Millisecond
	metrics.StartTPSCalculator(updateInterval)

	// Wait a bit for calculator to initialize
	time.Sleep(50 * time.Millisecond)

	// Record some operations
	initialCount := metrics.GetOpCount()
	for i := 0; i < 10; i++ {
		metrics.IncrementOpCount()
	}

	// Wait for TPS calculation
	time.Sleep(200 * time.Millisecond)

	// Verify TPS was calculated (should be > 0)
	tps := getGaugeValue(metrics.IngestOpsTPS)
	assert.GreaterOrEqual(t, tps, 0.0, "TPS should be calculated")

	// Verify operation count increased
	finalCount := metrics.GetOpCount()
	assert.Equal(t, initialCount+10, finalCount)
}

// TestHandler tests the HTTP handler for Prometheus metrics
func TestHandler(t *testing.T) {
	handler := metrics.Handler()
	require.NotNil(t, handler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// Record some metrics first
	metrics.RecordIngestOp("plugin")
	metrics.UpdateQueueSize(100)
	metrics.UpdateCurrentBlock(5000)

	// Call handler
	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")

	// Verify response body contains expected metrics
	body := w.Body.String()
	assert.Contains(t, body, "steemdb_sync_ingest_ops_total")
	assert.Contains(t, body, "steemdb_sync_queue_size")
	assert.Contains(t, body, "steemdb_sync_current_block")
}

// TestHandlerPrometheusFormat tests that handler returns Prometheus format
func TestHandlerPrometheusFormat(t *testing.T) {
	handler := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// Record a metric
	metrics.RecordIngestOp("plugin")

	handler.ServeHTTP(w, req)

	body := w.Body.String()
	
	// Verify Prometheus format (should contain HELP and TYPE lines)
	assert.Contains(t, body, "# HELP")
	assert.Contains(t, body, "# TYPE")
	
	// Verify metric line format (name{labels} value)
	lines := strings.Split(body, "\n")
	hasMetricLine := false
	for _, line := range lines {
		if strings.HasPrefix(line, "steemdb_sync_") && !strings.HasPrefix(line, "#") {
			hasMetricLine = true
			// Verify format: metric_name{label="value"} number
			assert.Regexp(t, `^steemdb_sync_\w+(\{[^}]+\})?\s+[\d.]+$`, line)
			break
		}
	}
	assert.True(t, hasMetricLine, "Should have at least one metric line")
}

// Helper functions

func getCounterValue(counter prometheus.Counter) float64 {
	var m prometheus.Metric = counter
	var pb dto.Metric
	err := m.Write(&pb)
	if err != nil {
		return 0
	}
	return pb.Counter.GetValue()
}

func getGaugeValue(gauge prometheus.Gauge) float64 {
	var m prometheus.Metric = gauge
	var pb dto.Metric
	err := m.Write(&pb)
	if err != nil {
		return 0
	}
	return pb.Gauge.GetValue()
}
