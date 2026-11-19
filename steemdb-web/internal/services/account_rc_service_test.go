package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// rcServiceTestLogger keeps the service tests quiet.
func rcServiceTestLogger() utils.Logger {
	logger, err := utils.NewLogger(utils.LogConfig{Level: "error"})
	if err != nil {
		panic(err)
	}
	return logger
}

// newRCService builds an AccountService pointed at the test server. The Mongo
// handle stays nil: GetAccountRC only exercises the steemClient + cache path.
// The backoff unit is shrunk so retry-loop tests run in milliseconds; the
// per-attempt timeout keeps its production value because the test handlers
// answer instantly.
func newRCService(srvURL string) *AccountService {
	client := steem.NewClient([]string{srvURL}, rcServiceTestLogger())
	client.BackoffUnitForTest(time.Millisecond)
	return NewAccountService(nil, client, rcServiceTestLogger())
}

// TestGetAccountRC_SingleFlightMergesConcurrentMisses asserts that a burst of
// concurrent requests for one cache-cold account produces exactly one rc_api
// round trip — the singleflight behavior the labs valuation cache set as the
// repo precedent.
func TestGetAccountRC_SingleFlightMergesConcurrentMisses(t *testing.T) {
	var upstreamCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		time.Sleep(20 * time.Millisecond) // widen the miss window
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[` +
			`{"account":"steemit","max_rc":"5642488170045",` +
			`"rc_manabar":{"current_mana":"5642263478103","last_update_time":1699771572}}]}}`))
	}))
	defer srv.Close()

	svc := newRCService(srv.URL)

	const burst = 20
	var wg sync.WaitGroup
	errs := make([]error, burst)
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rc, err := svc.GetAccountRC(context.Background(), "steemit")
			if err == nil && (rc == nil || rc.Account != "steemit") {
				err = errors.New("unexpected rc payload")
			}
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	if n := upstreamCalls.Load(); n != 1 {
		t.Errorf("upstream calls = %d, want 1 (singleflight must merge concurrent misses)", n)
	}
}

// TestGetAccountRC_NegativeCacheSuppressesRefetch asserts that a not-found
// answer is negatively cached: the second lookup must not reach the upstream
// node again within the TTL.
func TestGetAccountRC_NegativeCacheSuppressesRefetch(t *testing.T) {
	var upstreamCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[]}}`))
	}))
	defer srv.Close()

	svc := newRCService(srv.URL)

	for i := 0; i < 3; i++ {
		if _, err := svc.GetAccountRC(context.Background(), "ghost"); !errors.Is(err, ErrRCAccountNotFound) {
			t.Fatalf("lookup %d: err = %v, want ErrRCAccountNotFound", i, err)
		}
	}
	if n := upstreamCalls.Load(); n != 1 {
		t.Errorf("upstream calls = %d, want 1 (not-found must be negatively cached)", n)
	}
}

// TestGetAccountRC_UpstreamFailureIsNotCached asserts that RPC failures are
// retried on the next request instead of being pinned by the cache.
func TestGetAccountRC_UpstreamFailureIsNotCached(t *testing.T) {
	var upstreamCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the first 8 calls: two full client retry loops (the client
		// makes rpcMaxRetries+1 = 4 attempts per GetAccountRC lookup), so
		// the first two lookups exhaust every attempt and error out.
		if upstreamCalls.Add(1) <= 8 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[` +
			`{"account":"steemit","max_rc":"10","rc_manabar":{"current_mana":"5","last_update_time":1}}]}}`))
	}))
	defer srv.Close()

	svc := newRCService(srv.URL)
	ctx := context.Background()

	// Each lookup exhausts the client's retry loop against one node, then
	// errors — and must not poison the cache.
	for i := 0; i < 2; i++ {
		if _, err := svc.GetAccountRC(ctx, "steemit"); err == nil {
			t.Fatalf("lookup %d: want upstream error", i)
		}
	}
	rc, err := svc.GetAccountRC(ctx, "steemit")
	if err != nil {
		t.Fatalf("third lookup: %v (recovery must be visible once the node answers)", err)
	}
	if rc == nil || rc.CurrentMana != 5 || rc.MaxMana != 10 {
		t.Errorf("rc = %+v, want mana 5/10", rc)
	}
}
