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
)

// rcTestServer builds an httptest server that counts requests and answers
// with the handler's response. It pairs with newFastRetryClient so retry
// paths run in milliseconds.
func rcTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func rcRequestBody(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	return body
}

// TestGetRCAccount_WireShapeAndDecode asserts the wire contract this client
// exists for: rc_api requires NAMED params — an object, not a positional
// array — and the result decodes into RCAccount.
func TestGetRCAccount_WireShapeAndDecode(t *testing.T) {
	srv, count := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := rcRequestBody(t, r)
		if body["method"] != "rc_api.find_rc_accounts" {
			t.Errorf("method = %v, want rc_api.find_rc_accounts", body["method"])
		}
		params, ok := body["params"].(map[string]interface{})
		if !ok {
			t.Fatalf("params must be a named object, got %T", body["params"])
		}
		accounts, ok := params["accounts"].([]interface{})
		if !ok || len(accounts) != 1 || accounts[0] != "steemit" {
			t.Errorf("params.accounts = %v, want [steemit]", params["accounts"])
		}

		w.Header().Set("Content-Type", "application/json")
		// Shape captured live from api.steemit.com (account "steemit").
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[` +
			`{"account":"steemit","max_rc":"5642488170045",` +
			`"rc_manabar":{"current_mana":"5642263478103","last_update_time":1699771572}}]}}`))
	})

	c := newFastRetryClient([]string{srv.URL})
	rc, err := c.GetRCAccount(context.Background(), "steemit")
	if err != nil {
		t.Fatalf("GetRCAccount: %v", err)
	}
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1", count.Load())
	}
	if rc == nil {
		t.Fatal("rc is nil, want decoded account")
	}
	if rc.Account != "steemit" || rc.MaxRC != "5642488170045" ||
		rc.RCManabar.CurrentMana != "5642263478103" || rc.RCManabar.LastUpdateTime != 1699771572 {
		t.Errorf("decoded rc = %+v, want live steemit values", rc)
	}
}

// TestGetRCAccount_EmptyResultMeansNotFound asserts the (nil, nil) convention
// for an account absent from the chain.
func TestGetRCAccount_EmptyResultMeansNotFound(t *testing.T) {
	srv, _ := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[]}}`))
	})

	c := newFastRetryClient([]string{srv.URL})
	rc, err := c.GetRCAccount(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("GetRCAccount: %v (want nil error for empty result)", err)
	}
	if rc != nil {
		t.Errorf("rc = %+v, want nil", rc)
	}
}

// TestGetRCAccount_JSONRPCError surfaces the node's error message instead of
// treating the 200 envelope as success.
func TestGetRCAccount_JSONRPCError(t *testing.T) {
	srv, _ := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"Assert Exception: boom"}}`))
	})

	c := newFastRetryClient([]string{srv.URL})
	rc, err := c.GetRCAccount(context.Background(), "steemit")
	if err == nil {
		t.Fatal("want error for JSON-RPC error envelope")
	}
	if rc != nil {
		t.Errorf("rc = %+v, want nil", rc)
	}
	if !strings.Contains(err.Error(), "Assert Exception: boom") {
		t.Errorf("error should carry the node message, got: %v", err)
	}
}

// TestGetRCAccount_HTTPStatusAlwaysFails asserts the retry loop exhausts
// rpcMaxRetries+1 attempts against a node that keeps answering non-200.
func TestGetRCAccount_HTTPStatusAlwaysFails(t *testing.T) {
	srv, count := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	c := newFastRetryClient([]string{srv.URL})
	rc, err := c.GetRCAccount(context.Background(), "steemit")
	if err == nil {
		t.Fatal("want error when every attempt answers non-200")
	}
	if rc != nil {
		t.Errorf("rc = %+v, want nil", rc)
	}
	if want := int64(rpcMaxRetries + 1); count.Load() != want {
		t.Errorf("request count = %d, want %d (all attempts against one node)",
			count.Load(), want)
	}
	if !strings.Contains(err.Error(), "rc_api http status 502") {
		t.Errorf("error should name the status, got: %v", err)
	}
}

// TestGetRCAccount_RetriesThenSucceeds asserts the rotation retry recovers a
// transient failure without surfacing it to the caller.
func TestGetRCAccount_RetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int64
	srv, _ := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"rc_accounts":[` +
			`{"account":"ety001","max_rc":"81551146922851",` +
			`"rc_manabar":{"current_mana":"5308005006098","last_update_time":1795073355}}]}}`))
	})

	c := newFastRetryClient([]string{srv.URL})
	rc, err := c.GetRCAccount(context.Background(), "ety001")
	if err != nil {
		t.Fatalf("GetRCAccount: %v (transient failure must be retried away)", err)
	}
	if rc == nil || rc.Account != "ety001" {
		t.Errorf("rc = %+v, want decoded ety001", rc)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("request count = %d, want 2 (one failure + one success)", n)
	}
}

// TestGetRCAccount_ContextCancelled asserts a cancelled context short-circuits
// the retry loop instead of burning through every attempt.
func TestGetRCAccount_ContextCancelled(t *testing.T) {
	srv, count := rcTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	c := newFastRetryClient([]string{srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rc, err := c.GetRCAccount(ctx, "steemit")
	if err == nil {
		t.Fatal("want error for an already-cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if rc != nil {
		t.Errorf("rc = %+v, want nil", rc)
	}
	if count.Load() != 0 {
		t.Errorf("request count = %d, want 0 (cancelled context must not reach the node)",
			count.Load())
	}
}
