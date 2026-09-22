package steem

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steemit/steemdb/web/pkg/utils"
)

func testLogger() utils.Logger {
	logger, err := utils.NewLogger(utils.LogConfig{Level: "error"})
	if err != nil {
		panic(err)
	}
	return logger
}

// newFastRetryClient returns a Client whose per-attempt timeout and backoff
// are shrunk so timeout/retry tests run in milliseconds instead of the
// production 10s/1s values. Note these values do NOT preserve the production
// ratio (they would never trip a shared budget even if one existed), so tests
// that must prove budget-free retry semantics use
// newProductionRatioClient below instead.
func newFastRetryClient(nodes []string) *Client {
	c := NewClient(nodes, testLogger())
	c.attemptTimeout = 50 * time.Millisecond
	c.backoffUnit = time.Millisecond
	return c
}

// rpcProductionScale divides the production constants uniformly (1/40) so
// the production-ratio tests run in ~1.2s instead of 46s while preserving
// every relation between them: attemptTimeout == 10*backoffUnit, and the
// shared budget the retry-loop fix removed equaled rpcAttemptTimeout. Any
// reintroduced shared wall-clock budget proportional to these constants
// (budget == attemptTimeout, or budget == 10*backoffUnit) is exhausted by
// the first hung attempt and must fail those tests.
const rpcProductionScale = 40

// newProductionRatioClient returns a Client whose per-attempt timeout and
// backoff unit are the production constants scaled by 1/rpcProductionScale,
// keeping the production ratio intact.
func newProductionRatioClient(nodes []string) *Client {
	c := NewClient(nodes, testLogger())
	c.attemptTimeout = rpcAttemptTimeout / rpcProductionScale // 250ms
	c.backoffUnit = rpcBackoffUnit / rpcProductionScale       // 25ms
	return c
}

func TestCallWithAttemptTimeoutReturnsValueWhenFast(t *testing.T) {
	got, err := callWithAttemptTimeout(500*time.Millisecond, func() (string, error) {
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Fatalf("callWithAttemptTimeout() = (%q, %v), want (%q, nil)", got, err, "ok")
	}
}

func TestCallWithAttemptTimeoutAbandonsHungCall(t *testing.T) {
	start := time.Now()
	_, err := callWithAttemptTimeout(50*time.Millisecond, func() (string, error) {
		time.Sleep(2 * time.Second)
		return "late", nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrAttemptTimeout) {
		t.Fatalf("err = %v, want ErrAttemptTimeout", err)
	}
	if elapsed > time.Second {
		t.Fatalf("call took %v, want it to abandon after ~50ms", elapsed)
	}
}

func TestGetDynamicGlobalPropertiesBoundedAgainstHungNode(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// Simulate a node that accepts the connection but never answers in
		// time. Sleep long enough to outlive the client's attempt timeout
		// but still let server.Close() finish promptly.
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	client := newFastRetryClient([]string{server.URL})

	start := time.Now()
	props, err := client.GetDynamicGlobalProperties()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error from hung node, got props %+v", props)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("logical call took %v, want it bounded well below the SDK's own 30s HTTP timeout", elapsed)
	}

	// rpcMaxRetries+1 attempts must each have been attempted (and each
	// abandoned by the per-attempt timeout), proving retries still run.
	// Abandoned attempts are still in flight when the logical call returns,
	// so wait (bounded) for the last request to arrive before asserting.
	deadline := time.Now().Add(2 * time.Second)
	for requests.Load() < int32(rpcMaxRetries+1) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := requests.Load(); got != int32(rpcMaxRetries+1) {
		t.Fatalf("server saw %d requests, want %d", got, rpcMaxRetries+1)
	}
}

func TestGetDynamicGlobalPropertiesRotatesToHealthyNode(t *testing.T) {
	// Node A never answers in time; node B answers immediately. Whichever
	// node the client starts on, the call must succeed via rotation.
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer hung.Close()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      1,
			"jsonrpc": "2.0",
			"result": map[string]interface{}{
				"head_block_number":           123456,
				"last_irreversible_block_num": 123450,
				"current_witness":             "ety001",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer healthy.Close()

	client := newFastRetryClient([]string{hung.URL, healthy.URL})

	props, err := client.GetDynamicGlobalProperties()
	if err != nil {
		t.Fatalf("GetDynamicGlobalProperties() error = %v, want success via node rotation", err)
	}
	if props.HeadBlockNumber != 123456 {
		t.Fatalf("HeadBlockNumber = %d, want 123456", props.HeadBlockNumber)
	}
	if props.LastIrreversibleBlockNum != 123450 {
		t.Fatalf("LastIrreversibleBlockNum = %d, want 123450", props.LastIrreversibleBlockNum)
	}
}

// TestHungNodeRunsAllAttemptsAtProductionRatio proves the retry loop is free
// of any shared wall-clock budget, using the production constants scaled to
// 1/40 (attemptTimeout=250ms, backoffUnit=25ms — same 10:1 ratio, and the
// deleted budget equaled the attempt timeout). This is the parameter regime
// the audit found missing: the fast 50ms/1ms tests pass even when a shared
// budget cuts the retries, because such a budget never expires there. Here a
// reintroduced budget proportional to the constants (== attemptTimeout, or
// == 10*backoffUnit) is exhausted by the first hung attempt, the loop exits
// after a single attempt with a bare context.DeadlineExceeded, and every
// assertion below fails.
func TestHungNodeRunsAllAttemptsAtProductionRatio(t *testing.T) {
	// Two hung nodes: handlers block until the test releases them, so every
	// attempt genuinely hangs for its full per-attempt timeout, exactly like
	// a stalled production node.
	release := make(chan struct{})
	newHungServer := func(counter *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			<-release
		}))
	}
	var nodeA, nodeB atomic.Int32
	hungA := newHungServer(&nodeA)
	hungB := newHungServer(&nodeB)
	// Deferred in LIFO order: release the handlers first, then Close() (which
	// waits for outstanding handlers) can drain them promptly.
	defer hungB.Close()
	defer hungA.Close()
	defer close(release)

	client := newProductionRatioClient([]string{hungA.URL, hungB.URL})

	start := time.Now()
	_, err := client.GetDynamicGlobalProperties()
	elapsed := time.Since(start)

	// All attempts must have executed against hung nodes.
	deadline := time.Now().Add(2 * time.Second)
	for nodeA.Load()+nodeB.Load() < int32(rpcMaxRetries+1) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := nodeA.Load() + nodeB.Load(); got != int32(rpcMaxRetries+1) {
		t.Fatalf("hung servers saw %d requests total, want %d — a shared budget cut the retries (regression)", got, rpcMaxRetries+1)
	}

	// Rotation must have happened: with two nodes and one switch after every
	// failure, each node serves exactly half the attempts.
	if nodeA.Load() == 0 || nodeB.Load() == 0 {
		t.Fatalf("no rotation: node A saw %d, node B saw %d requests, want both > 0", nodeA.Load(), nodeB.Load())
	}

	// The 46s worst case, scaled by 1/40: four sequential hung attempts each
	// burn their full per-attempt timeout, so elapsed >= 4*attemptTimeout
	// (the budget regression finishes after ~1 attempt). The whole logical
	// call must also stay within attempts*timeout + 6*backoffUnit plus
	// scheduling slack.
	timeout := rpcAttemptTimeout / rpcProductionScale
	unit := rpcBackoffUnit / rpcProductionScale
	if elapsed < 4*timeout {
		t.Fatalf("elapsed %v, want >= 4*attemptTimeout (%v) — retries were cut short", elapsed, 4*timeout)
	}
	if want := time.Duration(rpcMaxRetries+1)*timeout + 6*unit; elapsed > want+time.Second {
		t.Fatalf("elapsed %v, want <= worst case %v + 1s scheduling slack", elapsed, want)
	}

	// Error semantics: a wrapped ErrAttemptTimeout naming the hung node, and
	// never a bare context.DeadlineExceeded leaking from an internal budget.
	if !errors.Is(err, ErrAttemptTimeout) {
		t.Fatalf("err = %v, want it to wrap ErrAttemptTimeout", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want no bare context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), hungA.URL) && !strings.Contains(err.Error(), hungB.URL) {
		t.Fatalf("err = %v, want it to name one of the hung nodes", err)
	}
}

func TestGetBlockSuccessPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      1,
			"jsonrpc": "2.0",
			"result": map[string]interface{}{
				"previous":          "0000000000000000000000000000000000000000",
				"timestamp":         "2026-09-22T00:00:00",
				"witness":           "ety001",
				"transactions":      []interface{}{},
				"witness_signature": "sig",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newFastRetryClient([]string{server.URL})

	block, err := client.GetBlock(42)
	if err != nil {
		t.Fatalf("GetBlock(42) error = %v, want success", err)
	}
	if block.Number != 42 {
		t.Fatalf("block.Number = %d, want 42", block.Number)
	}
	if block.Witness != "ety001" {
		t.Fatalf("block.Witness = %q, want %q", block.Witness, "ety001")
	}
}
