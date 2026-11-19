package services

import (
	"math"
	"testing"

	"github.com/steemit/steemdb/web/pkg/steem"
)

// TestBuildAccountRC covers the rc_api payload projection: normal values,
// zero max, and unparsable mana strings (degrade to 0, not an error).
func TestBuildAccountRC(t *testing.T) {
	tests := []struct {
		name     string
		rc       steem.RCAccount
		wantCurr int64
		wantMax  int64
		wantPct  float64
	}{
		{
			name: "normal values",
			// Values captured live from api.steemit.com (account "steemit").
			rc: steem.RCAccount{
				Account:   "steemit",
				RCManabar: steem.Manabar{CurrentMana: "5642263478103", LastUpdateTime: 1699771572},
				MaxRC:     "5642488170045",
			},
			wantCurr: 5642263478103,
			wantMax:  5642488170045,
			wantPct:  float64(5642263478103) / float64(5642488170045) * 100,
		},
		{
			name: "zero max yields zero percentage",
			rc: steem.RCAccount{
				Account:   "fresh",
				RCManabar: steem.Manabar{CurrentMana: "0", LastUpdateTime: 1},
				MaxRC:     "0",
			},
			wantCurr: 0,
			wantMax:  0,
			wantPct:  0,
		},
		{
			name: "unparsable mana degrades to zero",
			rc: steem.RCAccount{
				Account:   "weird",
				RCManabar: steem.Manabar{CurrentMana: "not-a-number", LastUpdateTime: 2},
				MaxRC:     "",
			},
			wantCurr: 0,
			wantMax:  0,
			wantPct:  0,
		},
		{
			name: "negative mana string degrades to zero",
			rc: steem.RCAccount{
				Account:   "negative",
				RCManabar: steem.Manabar{CurrentMana: "-5", LastUpdateTime: 3},
				MaxRC:     "100",
			},
			wantCurr: 0,
			wantMax:  100,
			wantPct:  0,
		},
		{
			// 1e19 is within uint64 range but above MaxInt64, so it
			// exercises the saturation branch — a value beyond MaxUint64
			// would take the parse-error path instead.
			name: "value above MaxInt64 saturates instead of reading as zero",
			rc: steem.RCAccount{
				Account:   "huge",
				RCManabar: steem.Manabar{CurrentMana: "10000000000000000000", LastUpdateTime: 4},
				MaxRC:     "10000000000000000000",
			},
			wantCurr: math.MaxInt64,
			wantMax:  math.MaxInt64,
			wantPct:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAccountRC(&tt.rc)
			if got.Account != tt.rc.Account {
				t.Errorf("Account = %q, want %q", got.Account, tt.rc.Account)
			}
			if got.CurrentMana != tt.wantCurr {
				t.Errorf("CurrentMana = %d, want %d", got.CurrentMana, tt.wantCurr)
			}
			if got.MaxMana != tt.wantMax {
				t.Errorf("MaxMana = %d, want %d", got.MaxMana, tt.wantMax)
			}
			if math.Abs(got.Percentage-tt.wantPct) > 1e-9 {
				t.Errorf("Percentage = %v, want %v", got.Percentage, tt.wantPct)
			}
			if got.LastUpdate != tt.rc.RCManabar.LastUpdateTime {
				t.Errorf("LastUpdate = %d, want %d", got.LastUpdate, tt.rc.RCManabar.LastUpdateTime)
			}
		})
	}
}
