package main

import (
	"context"
	"sort"
	"testing"

	"log"
)

type fakeVerifySource struct {
	createdNames []string
	accountIDs   []string
	landmark     string
	// referencedNow simulates creation ops that landed after startup
	// (the mid-run creation race); ConfirmNotCreated spares these.
	referencedNow map[string]bool
	deleted       []string
}

func (f *fakeVerifySource) StreamCreatedNames(ctx context.Context, fn func(name string) error) error {
	for _, n := range f.createdNames {
		if err := fn(n); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeVerifySource) LastAccountID(ctx context.Context) (string, error) {
	if f.landmark != "" {
		return f.landmark, nil
	}
	if len(f.accountIDs) == 0 {
		return "", nil
	}
	return f.accountIDs[len(f.accountIDs)-1], nil
}

func (f *fakeVerifySource) StreamAccountIDs(ctx context.Context, landmark string, fn func(id string) error) error {
	for _, id := range f.accountIDs {
		if id > landmark {
			continue
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeVerifySource) DeleteAccounts(ctx context.Context, ids []string) (int64, error) {
	f.deleted = append(f.deleted, ids...)
	return int64(len(ids)), nil
}

func (f *fakeVerifySource) ConfirmNotCreated(ctx context.Context, ids []string) ([]string, error) {
	var confirmed []string
	for _, id := range ids {
		if !f.referencedNow[id] {
			confirmed = append(confirmed, id)
		}
	}
	return confirmed, nil
}

func TestVerifyAccountsReVerificationSparesMidRunCreations(t *testing.T) {
	// "newacct" was not in the startup created set, but its creation op
	// landed while the scan ran — the pre-delete re-verification must spare
	// it even though it looks like a phantom.
	src := &fakeVerifySource{
		createdNames:  []string{"alice"},
		accountIDs:    []string{"alice", "newacct", "p1"},
		referencedNow: map[string]bool{"newacct": true},
	}
	res, err := verifyAccounts(context.Background(), src, 500, false, log.Default())
	if err != nil {
		t.Fatalf("verifyAccounts: %v", err)
	}
	if res.Phantom != 2 {
		t.Errorf("phantom = %d, want 2", res.Phantom)
	}
	if res.Deleted != 1 || len(src.deleted) != 1 || src.deleted[0] != "p1" {
		t.Errorf("deleted = %v, want [p1] only (mid-run created account spared)", src.deleted)
	}
}

func TestVerifyAccountsDryRun(t *testing.T) {
	src := &fakeVerifySource{
		createdNames: []string{"alice", "bob"},
		accountIDs:   []string{"alice", "bob", "carol-phantom", "ubmit.html"},
	}
	res, err := verifyAccounts(context.Background(), src, 500, true, log.Default())
	if err != nil {
		t.Fatalf("verifyAccounts: %v", err)
	}
	if res.Checked != 4 || res.Phantom != 2 {
		t.Errorf("checked=%d phantom=%d, want 4/2", res.Checked, res.Phantom)
	}
	if res.Deleted != 0 || len(src.deleted) != 0 {
		t.Error("dry run must not delete")
	}
	if len(res.Samples) != 2 {
		t.Errorf("samples = %v, want 2", res.Samples)
	}
}

func TestVerifyAccountsDeletes(t *testing.T) {
	src := &fakeVerifySource{
		createdNames: []string{"alice"},
		accountIDs:   []string{"alice", "p1", "p2", "p3"},
	}
	res, err := verifyAccounts(context.Background(), src, 2, false, log.Default())
	if err != nil {
		t.Fatalf("verifyAccounts: %v", err)
	}
	if res.Deleted != 3 {
		t.Errorf("deleted = %d, want 3", res.Deleted)
	}
	sort.Strings(src.deleted)
	want := []string{"p1", "p2", "p3"}
	for i, id := range want {
		if src.deleted[i] != id {
			t.Errorf("deleted[%d] = %q, want %q", i, src.deleted[i], id)
		}
	}
}

func TestVerifyAccountsGenesisAlwaysKept(t *testing.T) {
	src := &fakeVerifySource{
		createdNames: nil,
		accountIDs:   []string{"initminer", "miners", "null", "temp", "phantom"},
	}
	res, err := verifyAccounts(context.Background(), src, 500, true, log.Default())
	if err != nil {
		t.Fatalf("verifyAccounts: %v", err)
	}
	if res.Phantom != 1 || res.Samples[0] != "phantom" {
		t.Errorf("genesis accounts must be kept: phantom=%d samples=%v", res.Phantom, res.Samples)
	}
	if res.CreatedN != len(genesisAccounts) {
		t.Errorf("created set = %d, want %d (genesis only)", res.CreatedN, len(genesisAccounts))
	}
}

func TestVerifyAccountsLandmarkCutoff(t *testing.T) {
	src := &fakeVerifySource{
		createdNames: []string{"alice"},
		// landmark is "bob" — "zzz-phantom" sorts after it and must be skipped
		accountIDs: []string{"alice", "bob", "zzz-phantom"},
		landmark:   "bob",
	}
	res, err := verifyAccounts(context.Background(), src, 500, true, log.Default())
	if err != nil {
		t.Fatalf("verifyAccounts: %v", err)
	}
	if res.Checked != 2 || res.Phantom != 1 {
		t.Errorf("checked=%d phantom=%d, want 2/1 (ids past the landmark skipped)", res.Checked, res.Phantom)
	}
}
