package services

import (
	"errors"
	"testing"
	"time"

	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

func txTestLogger() utils.Logger {
	logger, err := utils.NewLogger(utils.LogConfig{Level: "error"})
	if err != nil {
		panic(err)
	}
	return logger
}

// fakeTxRPC stubs the upstream node. The call counters let tests assert
// the TTL/single-flight behaviour of the LIB cache.
type fakeTxRPC struct {
	tx      map[string]interface{}
	txErr   error
	txCalls int

	dgp      *steem.DynamicGlobalProperties
	dgpErr   error
	dgpCalls int
}

func (f *fakeTxRPC) GetTransaction(txID string) (map[string]interface{}, error) {
	f.txCalls++
	if f.txErr != nil {
		return nil, f.txErr
	}
	return f.tx, nil
}

func (f *fakeTxRPC) GetDynamicGlobalProperties() (*steem.DynamicGlobalProperties, error) {
	f.dgpCalls++
	if f.dgpErr != nil {
		return nil, f.dgpErr
	}
	return f.dgp, nil
}

// TestIncludedBlockNum covers the shape half of the irreversibility gate:
// only documents shaped like a real, included transaction (positive integer
// block_num) can ever be served or pinned. JSON numbers decode as float64,
// which is the shape the RPC client produces.
func TestIncludedBlockNum(t *testing.T) {
	tests := []struct {
		name string
		tx   map[string]interface{}
		want bool
	}{
		{
			name: "included transaction (float64 block_num)",
			tx:   map[string]interface{}{"block_num": float64(109832561)},
			want: true,
		},
		{
			name: "block_num zero",
			tx:   map[string]interface{}{"block_num": float64(0)},
			want: false,
		},
		{
			name: "unconfirmed shape (negative block_num)",
			tx:   map[string]interface{}{"block_num": float64(-1)},
			want: false,
		},
		{
			name: "no block_num at all",
			tx:   map[string]interface{}{"signatures": []interface{}{}},
			want: false,
		},
		{
			name: "block_num of wrong type",
			tx:   map[string]interface{}{"block_num": "109832561"},
			want: false,
		},
		{
			name: "fractional block_num",
			tx:   map[string]interface{}{"block_num": float64(109832561.5)},
			want: false,
		},
		{
			name: "beyond uint32 rejected",
			tx:   map[string]interface{}{"block_num": float64(4294967296)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := includedBlockNum(tt.tx); got != tt.want {
				t.Errorf("includedBlockNum(%v) ok = %v, want %v", tt.tx, got, tt.want)
			}
		})
	}
}

// TestLastIrreversibleBlockNum covers the authoritative-LIB cache: one
// upstream fetch per TTL window, refresh picking up chain progress, and the
// fail-safe directions — a failed refresh serves the last known LIB (still a
// valid lower bound, so the gate only gets stricter) while a failure with no
// known value propagates (irreversibility cannot be proven).
func TestLastIrreversibleBlockNum(t *testing.T) {
	t.Run("fetches once per TTL window", func(t *testing.T) {
		s := &TxService{steemClient: &fakeTxRPC{
			dgp: &steem.DynamicGlobalProperties{LastIrreversibleBlockNum: 100},
		}, logger: txTestLogger()}

		first, err := s.lastIrreversibleBlockNum()
		if err != nil || first != 100 {
			t.Fatalf("first call = (%d, %v), want (100, nil)", first, err)
		}
		second, err := s.lastIrreversibleBlockNum()
		if err != nil || second != 100 {
			t.Fatalf("cached call = (%d, %v), want (100, nil)", second, err)
		}
		if calls := s.steemClient.(*fakeTxRPC).dgpCalls; calls != 1 {
			t.Fatalf("upstream calls = %d, want 1 (TTL cache)", calls)
		}
	})

	t.Run("expired cache refetches and tracks chain progress", func(t *testing.T) {
		fake := &fakeTxRPC{
			dgp: &steem.DynamicGlobalProperties{LastIrreversibleBlockNum: 100},
		}
		s := &TxService{steemClient: fake, logger: txTestLogger()}
		if _, err := s.lastIrreversibleBlockNum(); err != nil {
			t.Fatalf("initial fetch: %v", err)
		}

		fake.dgp.LastIrreversibleBlockNum = 200
		s.libAt = time.Now().Add(-2 * txLibCacheTTL) // force expiry

		got, err := s.lastIrreversibleBlockNum()
		if err != nil || got != 200 {
			t.Fatalf("refreshed call = (%d, %v), want (200, nil)", got, err)
		}
		if fake.dgpCalls != 2 {
			t.Fatalf("upstream calls = %d, want 2", fake.dgpCalls)
		}
	})

	t.Run("failed refresh serves the last known value", func(t *testing.T) {
		fake := &fakeTxRPC{
			dgp: &steem.DynamicGlobalProperties{LastIrreversibleBlockNum: 100},
		}
		s := &TxService{steemClient: fake, logger: txTestLogger()}
		if _, err := s.lastIrreversibleBlockNum(); err != nil {
			t.Fatalf("initial fetch: %v", err)
		}

		fake.dgpErr = errors.New("node down")
		s.libAt = time.Now().Add(-2 * txLibCacheTTL) // force expiry

		got, err := s.lastIrreversibleBlockNum()
		if err != nil || got != 100 {
			t.Fatalf("degraded call = (%d, %v), want (100, nil): a stale LIB is a conservative lower bound", got, err)
		}
	})

	t.Run("failure with no known value propagates", func(t *testing.T) {
		s := &TxService{steemClient: &fakeTxRPC{
			dgpErr: errors.New("node down"),
		}, logger: txTestLogger()}

		if _, err := s.lastIrreversibleBlockNum(); err == nil {
			t.Fatal("want error when LIB has never been fetched and the node fails")
		}
	})

	t.Run("empty properties with no known value propagates", func(t *testing.T) {
		s := &TxService{steemClient: &fakeTxRPC{}, logger: txTestLogger()}

		if _, err := s.lastIrreversibleBlockNum(); err == nil {
			t.Fatal("want error when the node returns no properties and no LIB is known")
		}
	})
}
