package services

import "testing"

// TestPostSortField covers the sort-key whitelist of the posts listing:
// only index-backed comment fields are offered (sync's indexInventory:
// created, net_votes, pending_payout_value, total_payout_value). comment is
// the largest collection, so any other key falls back to the default
// instead of producing an unindexed in-memory sort.
func TestPostSortField(t *testing.T) {
	tests := []struct {
		name   string
		sortBy string
		want   string
	}{
		{name: "created", sortBy: "created", want: "created"},
		{name: "net_votes", sortBy: "net_votes", want: "net_votes"},
		{name: "pending_payout_value", sortBy: "pending_payout_value", want: "pending_payout_value"},
		{name: "total_payout_value", sortBy: "total_payout_value", want: "total_payout_value"},
		{name: "empty falls back to default", sortBy: "", want: "created"},
		{name: "unindexed votes shorthand falls back", sortBy: "votes", want: "created"},
		{name: "unindexed author falls back", sortBy: "author", want: "created"},
		{name: "injection attempt falls back", sortBy: "created: {}; $where", want: "created"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postSortField(tt.sortBy); got != tt.want {
				t.Errorf("postSortField(%q) = %q, want %q", tt.sortBy, got, tt.want)
			}
		})
	}
}
