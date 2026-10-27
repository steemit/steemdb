package api

import (
	"testing"
	"time"
)

// TestParseLeaderboardDate pins the date resolution shared by the curation and
// author leaderboards: monthly accepts both YYYY-MM and YYYY-MM-DD (a date
// control that keeps its daily value while the grouping switches used to 400),
// and an omitted date defaults to the current UTC day/month rather than the
// server-local one.
func TestParseLeaderboardDate(t *testing.T) {
	tests := []struct {
		name     string
		grouping string
		dateStr  string
		want     time.Time
		wantErr  bool
	}{
		{name: "daily", grouping: "daily", dateStr: "2026-10-26", want: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)},
		{name: "monthly YYYY-MM", grouping: "monthly", dateStr: "2026-10", want: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{name: "monthly given a full date", grouping: "monthly", dateStr: "2026-10-26", want: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)},
		{name: "daily rejects a month", grouping: "daily", dateStr: "2026-10", wantErr: true},
		{name: "garbage", grouping: "daily", dateStr: "no-such-day", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLeaderboardDate(tt.grouping, tt.dateStr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseLeaderboardDate(%q, %q) = %v, want an error", tt.grouping, tt.dateStr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLeaderboardDate(%q, %q): %v", tt.grouping, tt.dateStr, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseLeaderboardDate(%q, %q) = %v, want %v", tt.grouping, tt.dateStr, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("location = %v, want UTC", got.Location())
			}
		})
	}

	// Empty date: today (UTC) for daily, the current UTC month for monthly.
	now := time.Now().UTC()
	got, err := parseLeaderboardDate("daily", "")
	if err != nil || !got.Equal(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)) {
		t.Errorf("daily default = %v (err %v), want today in UTC", got, err)
	}
	got, err = parseLeaderboardDate("monthly", "")
	if err != nil || !got.Equal(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("monthly default = %v (err %v), want the 1st of the current UTC month", got, err)
	}
}
