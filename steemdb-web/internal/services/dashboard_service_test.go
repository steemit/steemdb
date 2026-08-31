package services

import (
	"testing"

	"github.com/steemit/steemdb/web/pkg/steem"
)

func TestShouldUseUpstream(t *testing.T) {
	head := int64(109000000)
	freshProps := &steem.DynamicGlobalProperties{HeadBlockNumber: head}

	tests := []struct {
		name      string
		localHead int64
		props     *steem.DynamicGlobalProperties
		want      bool
	}{
		{
			name:      "local missing uses upstream",
			localHead: 0,
			props:     freshProps,
			want:      true,
		},
		{
			name:      "local far behind uses upstream",
			localHead: head - stalenessMargin - 1,
			props:     freshProps,
			want:      true,
		},
		{
			name:      "local exactly at margin serves local",
			localHead: head - stalenessMargin,
			props:     freshProps,
			want:      false,
		},
		{
			name:      "local ahead of head serves local",
			localHead: head + 100,
			props:     freshProps,
			want:      false,
		},
		{
			name:      "upstream unavailable degrades to local",
			localHead: 0,
			props:     nil,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseUpstream(tt.localHead, tt.props); got != tt.want {
				t.Errorf("shouldUseUpstream(localHead=%d, props=%v) = %v, want %v",
					tt.localHead, tt.props, got, tt.want)
			}
		})
	}
}
