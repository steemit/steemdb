package checker

import (
	"context"
	"log"
	"testing"
)

type fakeHealthStore struct {
	count         int64
	minBlock      uint32
	maxBlock      uint32
	missingOld    int64
	missingNew    int64
	invalidIDs    []string
	stubCount     int64
	blockStatsErr error
}

func (f *fakeHealthStore) BlockStats(ctx context.Context) (int64, uint32, uint32, error) {
	return f.count, f.minBlock, f.maxBlock, f.blockStatsErr
}

func (f *fakeHealthStore) CountOperationsMissingAccounts(ctx context.Context, fromBlock, toBlock uint32) (int64, error) {
	if fromBlock == 1 {
		return f.missingOld, nil
	}
	return f.missingNew, nil
}

func (f *fakeHealthStore) InvalidAccountIDs(ctx context.Context) ([]string, error) {
	return f.invalidIDs, nil
}

func (f *fakeHealthStore) CountAccountStubs(ctx context.Context) (int64, error) {
	return f.stubCount, nil
}

func TestBlockContinuityCheck(t *testing.T) {
	cases := []struct {
		name        string
		count       int64
		min, max    uint32
		wantHealthy bool
	}{
		{"continuous", 100, 1, 100, true},
		{"gap", 90, 1, 100, false},
		{"empty", 0, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeHealthStore{count: tc.count, minBlock: tc.min, maxBlock: tc.max}
			f, err := (&blockContinuityCheck{store: store}).Run(context.Background())
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if f.Healthy != tc.wantHealthy {
				t.Errorf("Healthy = %v, want %v (summary: %s)", f.Healthy, tc.wantHealthy, f.Summary)
			}
			if f.RepairMode != "blocks" {
				t.Errorf("RepairMode = %q, want blocks", f.RepairMode)
			}
		})
	}
}

func TestOperationsAccountsCheck(t *testing.T) {
	cases := []struct {
		name                   string
		missingOld, missingNew int64
		wantHealthy            bool
	}{
		{"backfilled", 0, 0, true},
		{"old not backfilled", 500, 0, false},
		{"recent writer regression", 0, 3, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeHealthStore{count: 100, minBlock: 1, maxBlock: 5000, missingOld: tc.missingOld, missingNew: tc.missingNew}
			f, err := (&operationsAccountsCheck{store: store}).Run(context.Background())
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if f.Healthy != tc.wantHealthy {
				t.Errorf("Healthy = %v, want %v (summary: %s)", f.Healthy, tc.wantHealthy, f.Summary)
			}
		})
	}
}

func TestInvalidAccountIDsCheck(t *testing.T) {
	store := &fakeHealthStore{invalidIDs: []string{"bad..name", "-x"}}
	f, err := (&invalidAccountIDsCheck{store: store}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.Healthy {
		t.Error("expected unhealthy with invalid ids present")
	}
	if len(f.Details) != 2 {
		t.Errorf("Details = %d entries, want 2 samples", len(f.Details))
	}

	store.invalidIDs = nil
	f, _ = (&invalidAccountIDsCheck{store: store}).Run(context.Background())
	if !f.Healthy {
		t.Error("expected healthy with no invalid ids")
	}
}

func TestAccountStubsCheck(t *testing.T) {
	store := &fakeHealthStore{stubCount: 1042728}
	f, err := (&accountStubsCheck{store: store}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.Healthy {
		t.Error("expected unhealthy with phantom stubs present")
	}
	if f.RepairMode != "verify-accounts" {
		t.Errorf("RepairMode = %q, want verify-accounts", f.RepairMode)
	}
}

func TestRunHealthChecksAggregation(t *testing.T) {
	healthy := &fakeHealthStore{count: 100, minBlock: 1, maxBlock: 100}
	findings, allHealthy := RunHealthChecks(context.Background(), DefaultChecks(healthy), log.Default())
	if !allHealthy {
		t.Errorf("allHealthy = false, want true (%d findings)", len(findings))
	}
	if len(findings) != len(DefaultChecks(healthy)) {
		t.Errorf("findings = %d, want %d", len(findings), len(DefaultChecks(healthy)))
	}

	unhealthy := &fakeHealthStore{count: 90, minBlock: 1, maxBlock: 100, stubCount: 5}
	_, allHealthy = RunHealthChecks(context.Background(), DefaultChecks(unhealthy), log.Default())
	if allHealthy {
		t.Error("allHealthy = true, want false")
	}
}
