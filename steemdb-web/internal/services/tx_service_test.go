package services

import (
	"testing"
)

// TestCacheable covers the cache-admission guard: only confirmed, included
// transactions (block_num > 0) may be pinned forever. JSON numbers decode as
// float64, which is the shape the RPC client produces.
func TestCacheable(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheable(tt.tx); got != tt.want {
				t.Errorf("cacheable(%v) = %v, want %v", tt.tx, got, tt.want)
			}
		})
	}
}
