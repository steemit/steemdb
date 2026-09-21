package steem

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
// production 10s/1s values.
func newFastRetryClient(nodes []string) *Client {
	c := NewClient(nodes, testLogger())
	c.attemptTimeout = 50 * time.Millisecond
	c.backoffUnit = time.Millisecond
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
		t.Fatalf("GetBlock(42) error = %v", err)
	}
	if block.Number != 42 {
		t.Fatalf("block.Number = %d, want 42", block.Number)
	}
	if block.Witness != "ety001" {
		t.Fatalf("block.Witness = %q, want %q", block.Witness, "ety001")
	}
}
