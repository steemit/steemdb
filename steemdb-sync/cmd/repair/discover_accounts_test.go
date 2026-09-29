package main

import (
	"context"
	"log"
	"testing"
)

type fakeDiscoverSource struct {
	createdNames []string
	// id -> isStub
	accounts map[string]bool
	inserted []string
}

func (f *fakeDiscoverSource) StreamCreatedNames(ctx context.Context, fn func(name string) error) error {
	for _, n := range f.createdNames {
		if err := fn(n); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeDiscoverSource) StreamAccountIDs(ctx context.Context, fn func(id string, isStub bool) error) error {
	for id, isStub := range f.accounts {
		if err := fn(id, isStub); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeDiscoverSource) InsertAccountStubs(ctx context.Context, ids []string) (int64, error) {
	f.inserted = append(f.inserted, ids...)
	return int64(len(ids)), nil
}

func genesisPresent() map[string]bool {
	m := make(map[string]bool, len(genesisAccounts))
	for _, g := range genesisAccounts {
		m[g] = false
	}
	return m
}

func TestDiscoverAccountsInsertsMissing(t *testing.T) {
	accounts := genesisPresent()
	accounts["alice"] = false
	src := &fakeDiscoverSource{
		createdNames: []string{"alice", "bob", "carol"},
		accounts:     accounts,
	}
	res, err := discoverAccounts(context.Background(), src, 500, false, log.Default())
	if err != nil {
		t.Fatalf("discoverAccounts: %v", err)
	}
	if res.Missing != 2 || res.Inserted != 2 {
		t.Errorf("missing=%d inserted=%d, want 2/2", res.Missing, res.Inserted)
	}
	if res.CreatedN != 3+len(genesisAccounts) {
		t.Errorf("created set = %d, want %d", res.CreatedN, 3+len(genesisAccounts))
	}
}

func TestDiscoverAccountsDryRun(t *testing.T) {
	accounts := genesisPresent()
	accounts["alice"] = false
	src := &fakeDiscoverSource{
		createdNames: []string{"alice", "bob"},
		accounts:     accounts,
	}
	res, err := discoverAccounts(context.Background(), src, 500, true, log.Default())
	if err != nil {
		t.Fatalf("discoverAccounts: %v", err)
	}
	if res.Missing != 1 || res.Inserted != 0 || len(src.inserted) != 0 {
		t.Errorf("dry run must report but not insert: missing=%d inserted=%d", res.Missing, res.Inserted)
	}
}

func TestDiscoverAccountsAnomalyReport(t *testing.T) {
	src := &fakeDiscoverSource{
		createdNames: []string{"alice"},
		accounts: map[string]bool{
			"alice":         false, // full doc, created — fine
			"ghost-phantom": true,  // stub, not created — verify-accounts territory, not anomaly
			"ghost-fulldoc": false, // full doc, not created — anomaly
		},
	}
	res, err := discoverAccounts(context.Background(), src, 500, true, log.Default())
	if err != nil {
		t.Fatalf("discoverAccounts: %v", err)
	}
	// genesis names are in the created set but not in this fake collection
	if res.Missing != len(genesisAccounts) {
		t.Errorf("missing = %d, want %d (genesis accounts)", res.Missing, len(genesisAccounts))
	}
	if res.AnomalyCount != 1 || res.AnomalySamples[0] != "ghost-fulldoc" {
		t.Errorf("anomalies = %d %v, want 1 [ghost-fulldoc]", res.AnomalyCount, res.AnomalySamples)
	}
}
