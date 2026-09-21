package services

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/steemit/steemdb/web/internal/models"
)

// legacyRigidRow mirrors the pre-fix decode shape of the labs aggregations.
// It exists only to prove that rigid decoding of sync-written documents is
// structurally broken (P0-1/P0-2): unmarshalling the same BSON bytes that the
// tolerant shapes accept must fail here.
type legacyRigidRow struct {
	ID        bson.M           `bson:"_id"`
	Count     int              `bson:"count"`
	Instances []string         `bson:"instances"`
	Account   []models.Account `bson:"account"`
}

// toxicAccountDoc returns an account document as written by steemdb-sync's
// refresher: raw RPC strings are preserved for withdrawn, curation_rewards,
// posting_rewards, proxied_vsf_votes and last_account_update, while
// refresher-converted fields (reputation, balances, dates) are numeric.
func toxicAccountDoc() bson.M {
	return bson.M{
		"_id":                 "alice",
		"name":                "alice",
		"reputation":          int64(6312294827984),
		"post_count":          int32(6407),
		"vesting_shares":      15281315.531,
		"balance":             2.432,
		"sbd_balance":         0.0,
		"created":             time.Date(2016, 8, 7, 6, 43, 54, 0, time.UTC),
		"last_post":           time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		"withdrawn":           "0",
		"to_withdraw":         0.0,
		"curation_rewards":    "9412993",
		"posting_rewards":     "19217671",
		"proxied_vsf_votes":   bson.A{"0", "0", "0", "0"},
		"last_account_update": "2024-03-01T00:00:00",
	}
}

// TestLabsLookupTolerantDecode proves the five labs endpoints' $lookup result
// survives documents with raw-RPC string fields: the tolerant []bson.M decode
// succeeds and projects a summary, while the legacy []models.Account rigid
// decode fails on the very same bytes (the P0-1 500).
func TestLabsLookupTolerantDecode(t *testing.T) {
	tests := []struct {
		name    string
		account bson.M
	}{
		{
			name:    "raw RPC strings preserved by the refresher",
			account: toxicAccountDoc(),
		},
		{
			// The same join after a hypothetical writer that stores the
			// poison fields natively must also keep working.
			name: "all fields natively typed",
			account: bson.M{
				"_id":                 "bob",
				"name":                "bob",
				"reputation":          int64(2581047289),
				"post_count":          int64(12),
				"vesting_shares":      float64(1001),
				"balance":             int32(7),
				"sbd_balance":         float64(0),
				"created":             time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
				"last_post":           time.Date(2026, 9, 2, 3, 4, 5, 0, time.UTC),
				"withdrawn":           int64(0),
				"curation_rewards":    int64(10),
				"posting_rewards":     int64(20),
				"proxied_vsf_votes":   bson.A{int64(0), int64(0)},
				"last_account_update": time.Date(2023, 5, 6, 7, 8, 9, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := bson.Marshal(bson.M{
				"_id":       bson.M{"user": "alice"},
				"count":     3,
				"instances": bson.A{1.5},
				"account":   bson.A{tt.account},
			})
			if err != nil {
				t.Fatalf("failed to marshal row: %v", err)
			}

			// The legacy rigid decode must fail on refresher-written docs.
			var rigid legacyRigidRow
			if err := bson.Unmarshal(raw, &rigid); err == nil {
				t.Errorf("legacy rigid decode unexpectedly succeeded; want it to fail (guards against reintroducing []models.Account)")
			}

			// The tolerant decode must succeed and project the summary.
			var row struct {
				ID        bson.M        `bson:"_id"`
				Account   []bson.M      `bson:"account"`
				Instances []interface{} `bson:"instances"`
			}
			if err := bson.Unmarshal(raw, &row); err != nil {
				t.Fatalf("tolerant decode failed: %v", err)
			}

			summary := accountSummaryFromLookup(row.Account)
			if summary == nil {
				t.Fatal("accountSummaryFromLookup returned nil for non-empty join result")
			}
			if want := tt.account["name"].(string); summary.Name != want {
				t.Errorf("summary.Name = %q, want %q", summary.Name, want)
			}
			if want := toFloat64(tt.account["reputation"]); float64(summary.Reputation) != want {
				t.Errorf("summary.Reputation = %d, want %.0f", summary.Reputation, want)
			}
			if want := toFloat64(tt.account["post_count"]); float64(summary.PostCount) != want {
				t.Errorf("summary.PostCount = %d, want %.0f", summary.PostCount, want)
			}
		})
	}
}

// TestAccountSummaryFromLookupEmpty covers the no-match join case.
func TestAccountSummaryFromLookupEmpty(t *testing.T) {
	if got := accountSummaryFromLookup(nil); got != nil {
		t.Errorf("accountSummaryFromLookup(nil) = %v, want nil", got)
	}
	if got := accountSummaryFromLookup([]bson.M{}); got != nil {
		t.Errorf("accountSummaryFromLookup(empty) = %v, want nil", got)
	}
}

// TestAccountSummaryFromLookupNameFallback covers stub account documents
// (no "name" field) whose identity lives in _id.
func TestAccountSummaryFromLookupNameFallback(t *testing.T) {
	summary := accountSummaryFromLookup([]bson.M{{"_id": "carol"}})
	if summary == nil {
		t.Fatal("accountSummaryFromLookup returned nil")
	}
	if summary.Name != "carol" {
		t.Errorf("summary.Name = %q, want %q (from _id)", summary.Name, "carol")
	}
}

// TestPowerUpInstancesCoercion proves the numeric vesting_deposit.amount
// decodes and coerces correctly (P0-2): steemdb-sync stores a plain float
// (mirroring legacy sync.py), never an asset string.
func TestPowerUpInstancesCoercion(t *testing.T) {
	tests := []struct {
		name      string
		instances bson.A
		want      []float64
	}{
		{
			name:      "doubles (current writer)",
			instances: bson.A{100.5, 0.001, 42.0},
			want:      []float64{100.5, 0.001, 42.0},
		},
		{
			name:      "mixed BSON numeric types",
			instances: bson.A{int64(7), int32(3), 2.25},
			want:      []float64{7, 3, 2.25},
		},
		{
			name:      "asset strings from older writers still parse",
			instances: bson.A{"100.000 STEEM", "0.500 STEEM"},
			want:      []float64{100, 0.5},
		},
		{
			name:      "empty",
			instances: bson.A{},
			want:      []float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := bson.Marshal(bson.M{
				"_id":       bson.M{"user": "alice"},
				"count":     len(tt.instances),
				"instances": tt.instances,
				"account":   bson.A{},
			})
			if err != nil {
				t.Fatalf("failed to marshal row: %v", err)
			}

			// Numeric amounts must break the legacy []string decode.
			hasNumeric := false
			for _, v := range tt.instances {
				switch v.(type) {
				case float64, int32, int64:
					hasNumeric = true
				}
			}
			if hasNumeric {
				var rigid legacyRigidRow
				if err := bson.Unmarshal(raw, &rigid); err == nil {
					t.Errorf("legacy []string instances decode unexpectedly succeeded; want it to fail on numeric amounts")
				}
			}

			var row powerUpRow
			if err := bson.Unmarshal(raw, &row); err != nil {
				t.Fatalf("tolerant decode failed: %v", err)
			}

			total := 0.0
			got := make([]float64, 0, len(row.Instances))
			for _, inst := range row.Instances {
				v := toFloat64(inst)
				got = append(got, v)
				total += v
			}

			if len(got) != len(tt.want) {
				t.Fatalf("coerced instances = %v, want %v", got, tt.want)
			}
			wantTotal := 0.0
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("instances[%d] = %v, want %v", i, got[i], w)
				}
				wantTotal += w
			}
			if total != wantTotal {
				t.Errorf("total = %v, want %v", total, wantTotal)
			}
		})
	}
}

// TestPendingPostsWindow guards the P0-3 fix: the created-time window must
// span [12.5 days ago, 7 days ago] with the earlier timestamp as the lower
// bound, so the range is non-empty.
func TestPendingPostsWindow(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	start, end := pendingPostsWindow(now)

	wantStart := now.AddDate(0, 0, -12).Add(-12 * time.Hour)
	wantEnd := now.AddDate(0, 0, -7)

	if !start.Equal(wantStart) {
		t.Errorf("window start = %v, want %v (12.5 days ago)", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("window end = %v, want %v (7 days ago)", end, wantEnd)
	}
	if !start.Before(end) {
		t.Errorf("window is empty: start %v must be before end %v", start, end)
	}
}

// TestToFloat64 covers the coercion helper used for aggregation output that
// may arrive as any BSON numeric type or as a string.
func TestToFloat64(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  float64
	}{
		{"float64", 12.5, 12.5},
		{"int64", int64(-3), -3},
		{"int32", int32(8), 8},
		{"int", 5, 5},
		{"asset string", "123.000 STEEM", 123},
		{"plain numeric string", "45.6", 45.6},
		{"garbage string", "not-a-number", 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toFloat64(tt.input); got != tt.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
