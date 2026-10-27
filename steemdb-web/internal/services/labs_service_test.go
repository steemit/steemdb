package services

import (
	"context"
	"errors"
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

// TestPendingPostsWindow guards the legacy-faithful pending window: the
// created-time range must span [7 days ago, 156 hours (6.5 days) ago] —
// the 12-hour review window before the 7-day cashout, matching legacy
// LabsController::pendingAction ($gte "-7 days", $lte "-156 hours").
func TestPendingPostsWindow(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	start, end := pendingPostsWindow(now)

	wantStart := now.AddDate(0, 0, -7)
	wantEnd := now.Add(-156 * time.Hour)

	if !start.Equal(wantStart) {
		t.Errorf("window start = %v, want %v (7 days ago)", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("window end = %v, want %v (156 hours / 6.5 days ago)", end, wantEnd)
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

// TestVestsToSteem guards the units contract of the power down figures: the
// chain stores VESTS, the page must show STEEM, and a missing props snapshot
// must pass the amount through instead of zeroing it.
func TestVestsToSteem(t *testing.T) {
	// Production snapshot, 2026-10-26: 201,658,879.271 STEEM over
	// 324,827,389,106.419916 VESTS → 1 STEEM ≈ 1610.78 VESTS.
	ratio := vestsToSteemRatio("201658879.271 STEEM", "324827389106.419916 VESTS")
	if ratio <= 0 {
		t.Fatalf("ratio = %v, want a positive STEEM-per-VEST rate", ratio)
	}
	if got := 1 / ratio; got < 1610 || got > 1612 {
		t.Errorf("1 STEEM = %.2f VESTS, want ≈1610.8", got)
	}

	// 348,792,400.290 VESTS must render as ≈216,536 STEEM, not 348 million.
	if got := vestsToSteem(348792400.290, ratio); got < 216000 || got > 217000 {
		t.Errorf("vestsToSteem(348792400.29) = %v STEEM, want ≈216536", got)
	}

	// Missing props: no rate, value untouched (never silently zero).
	if got := vestsToSteemRatio("", ""); got != 0 {
		t.Errorf("ratio without props = %v, want 0", got)
	}
	if got := vestsToSteem(27.5, 0); got != 27.5 {
		t.Errorf("vestsToSteem with no rate = %v, want the input 27.5", got)
	}
}

// TestRewardValuationSteemEquivalent pins the author-board ranking metric: all
// three payout legs are converted to STEEM, VESTS by the props ratio and SBD by
// the median price feed. Treating SBD as STEEM at par (2026-10: 1 SBD ≈ 9.6
// STEEM) or dropping VESTS (52.4% of one day's author rewards) misranks the
// board.
func TestRewardValuationSteemEquivalent(t *testing.T) {
	// Production snapshot 2026-10-26: 201,658,879.271 STEEM over
	// 324,827,389,106.419916 VESTS, and a median feed of
	// 58,455,023.535 SBD / 561,645,706.579 STEEM.
	valuation := rewardValuation{
		VestsRatio:  vestsToSteemRatio("201658879.271 STEEM", "324827389106.419916 VESTS"),
		SteemPerSBD: 561645706.579 / 58455023.535,
	}

	// 1,126,437.341679 VESTS ≈ 699.3 STEEM (the audit's chain cross-check).
	if got := valuation.steemEquivalent(1126437.341679, 0, 0); got < 698 || got > 700 {
		t.Errorf("vests leg = %v STEEM, want ≈699.3", got)
	}
	// A pure SBD payout is worth ~9.6x its face value in STEEM.
	if got := valuation.steemEquivalent(0, 0, 1); got < 9.5 || got > 9.7 {
		t.Errorf("1 SBD = %v STEEM, want ≈9.6 (never 1)", got)
	}
	// All three legs add up.
	if got, want := valuation.steemEquivalent(1126437.341679, 548.186, 0.5), 699.307+548.186+4.804; got < want-1 || got > want+1 {
		t.Errorf("total = %v STEEM, want ≈%v", got, want)
	}
}

// TestRewardValuationFailureBackoff pins the node-failure behaviour of the
// author-board valuation cache. The previous shape held a mutex across two RPC
// calls, so every concurrent request queued behind the same (up to 46s) refresh
// and a single sick node became an endpoint-wide pile-up. Now: the refresh is
// merged, failures are recorded and short-circuit the next attempt, and a
// last-good valuation keeps being served instead of failing the board.
func TestRewardValuationFailureBackoff(t *testing.T) {
	// No RPC client: every refresh fails, which is the "node unreachable" shape
	// minus the wait.
	svc := &LabsService{}

	start := time.Now()
	if _, err := svc.rewardValuation(context.Background()); err == nil {
		t.Fatal("cold cache with no client returned a valuation; want an error")
	}
	failedAt := svc.lastValuationFailure()
	if failedAt.IsZero() {
		t.Fatal("refresh failure was not recorded; every request would hit the node again")
	}
	if failedAt.Before(start) {
		t.Errorf("failure timestamp %v predates the attempt at %v", failedAt, start)
	}

	// Inside the retry window the call short-circuits instead of retrying.
	if _, err := svc.rewardValuation(context.Background()); !errors.Is(err, errValuationRecentFailure) {
		t.Errorf("second call error = %v, want errValuationRecentFailure", err)
	}

	// With a last-good valuation the same failure serves the stale rate: a
	// minute-stale conversion is within 0.05% of the truth, an error page is not.
	svc.valMu.Lock()
	svc.val = rewardValuation{VestsRatio: 1e-6, SteemPerSBD: 9.6}
	svc.valAt = time.Now().Add(-2 * labsValuationTTL)
	svc.valMu.Unlock()

	got, err := svc.rewardValuation(context.Background())
	if err != nil {
		t.Fatalf("stale valuation not served on a failed refresh: %v", err)
	}
	if got.SteemPerSBD != 9.6 || got.VestsRatio != 1e-6 {
		t.Errorf("stale valuation = %+v, want the cached {VestsRatio:1e-6 SteemPerSBD:9.6}", got)
	}
}

// TestRewardValuationServesFreshCache pins that a valuation inside
// labsValuationTTL is returned without any RPC: the author board must not put
// two public-node round trips inside every request.
func TestRewardValuationServesFreshCache(t *testing.T) {
	svc := &LabsService{}
	svc.valMu.Lock()
	svc.val = rewardValuation{VestsRatio: 2e-6, SteemPerSBD: 9.5}
	svc.valAt = time.Now()
	svc.valMu.Unlock()

	got, err := svc.rewardValuation(context.Background())
	if err != nil {
		t.Fatalf("fresh cache returned an error: %v", err)
	}
	if got.SteemPerSBD != 9.5 {
		t.Errorf("valuation = %+v, want the cached SteemPerSBD 9.5", got)
	}
	if !svc.lastValuationFailure().IsZero() {
		t.Error("a fresh cache hit recorded a failure")
	}
}

// TestClampLabsLimit pins the row-cap policy shared by the labs boards: a
// missing request gets the default, an oversized one is clamped, and a
// negative or zero value never means "no limit".
func TestClampLabsLimit(t *testing.T) {
	cases := []struct {
		requested, def, max, want int
	}{
		{0, 200, 500, 200},
		{-5, 200, 500, 200},
		{50, 200, 500, 50},
		{500, 200, 500, 500},
		{100000, 200, 500, 500},
	}
	for _, c := range cases {
		if got := clampLabsLimit(c.requested, c.def, c.max); got != c.want {
			t.Errorf("clampLabsLimit(%d, %d, %d) = %d, want %d", c.requested, c.def, c.max, got, c.want)
		}
	}
}

// TestPreviousPowerDownWindow pins the "(Last 7 Days)" window to a rolling
// 7x24h span. A day-aligned bound spans 8 UTC buckets, and because a power
// down batch repeats every 7 days the same batch then lands in the first and
// the last bucket, double-counting it in the total.
func TestPreviousPowerDownWindow(t *testing.T) {
	now := time.Date(2026, 10, 26, 23, 2, 0, 0, time.UTC)
	want := now.Add(-7 * 24 * time.Hour)

	got := previousPowerDownWindow(now)
	if !got.Equal(want) {
		t.Errorf("window start = %v, want %v (exactly 7x24h, not day-aligned)", got, want)
	}
	if got.Equal(now.AddDate(0, 0, -7).Truncate(24 * time.Hour)) {
		t.Error("window start is still day-aligned; it would span 8 UTC buckets")
	}
}

// TestBenefactorWindowStart pins the benefactor leaderboard to the 10 daily
// buckets ending today (UTC). The unbounded aggregation exceeded the $group
// memory cap (MongoDB code 292) and its ascending sort returned the oldest
// 10 days of the collection instead of the recent ones.
func TestBenefactorWindowStart(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "mid-day UTC",
			now:  time.Date(2026, 10, 26, 15, 50, 0, 0, time.UTC),
			want: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "local zone is normalised to UTC midnight",
			now:  time.Date(2026, 10, 27, 7, 30, 0, 0, time.FixedZone("CST", 8*3600)),
			want: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := benefactorWindowStart(tt.now)
			if !got.Equal(tt.want) {
				t.Errorf("benefactorWindowStart(%v) = %v, want %v", tt.now, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("window start location = %v, want UTC", got.Location())
			}
			// 0001-01-01 documents from a cold-start replay with an
			// uninitialised block timestamp must fall outside the window.
			if !got.After(time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("window start %v would admit pre-2017 garbage documents", got)
			}
		})
	}
}

// TestFlagsFromSnapshot guards the flags snapshot decode: steemdb-sync writes
// {author, count, voter_count, post_count, voters:[{voter,count}]} into
// status.flags-snapshot, and the API must expose the same fields (the old
// response shipped full flaggers/posts arrays the SPA never read).
func TestFlagsFromSnapshot(t *testing.T) {
	doc := bson.M{
		"_id": "flags-snapshot",
		"data": bson.A{
			bson.M{
				"author":      "appics-introduce",
				"count":       381430,
				"voter_count": 251,
				"post_count":  8234,
				"voters": bson.A{
					bson.M{"voter": "steemcleaners", "count": 900},
					bson.M{"voter": "spaminator", "count": 41},
				},
			},
			bson.M{
				"author":      "schoolofminnows",
				"count":       149100,
				"voter_count": 173,
				"post_count":  2956,
				"voters":      bson.A{},
			},
		},
	}

	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	var snapshot struct {
		ID   string              `bson:"_id"`
		Data []flagSnapshotEntry `bson:"data"`
	}
	if err := bson.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}
	if snapshot.ID != flagsSnapshotID {
		t.Errorf("snapshot._id = %q, want %q", snapshot.ID, flagsSnapshotID)
	}

	flags := flagsFromSnapshot(snapshot.Data)
	if len(flags) != 2 {
		t.Fatalf("flags length = %d, want 2", len(flags))
	}

	first := flags[0]
	if first.Author != "appics-introduce" || first.Count != 381430 {
		t.Errorf("first entry = %+v, want author=appics-introduce count=381430", first)
	}
	if first.VoterCount != 251 {
		t.Errorf("VoterCount = %d, want 251 (distinct voters)", first.VoterCount)
	}
	if first.PostCount != 8234 {
		t.Errorf("PostCount = %d, want 8234 (distinct posts)", first.PostCount)
	}
	if len(first.Voters) != 2 || first.Voters["steemcleaners"] != 900 || first.Voters["spaminator"] != 41 {
		t.Errorf("Voters = %v, want the top-flagger op counts", first.Voters)
	}

	second := flags[1]
	if len(second.Voters) != 0 {
		t.Errorf("second entry Voters = %v, want empty", second.Voters)
	}
}
