package model

import (
	"reflect"
	"testing"
)

func TestExtractAccounts(t *testing.T) {
	tests := []struct {
		name string
		op   map[string]interface{}
		want []string
	}{
		{
			name: "transfer has from and to",
			op: map[string]interface{}{
				"from":   "alice",
				"to":     "bob",
				"amount": "5.000 STEEM",
				"memo":   "hi",
			},
			want: []string{"alice", "bob"},
		},
		{
			name: "self transfer deduplicates",
			op: map[string]interface{}{
				"from": "alice",
				"to":   "alice",
			},
			want: []string{"alice"},
		},
		{
			name: "vote has voter and author",
			op: map[string]interface{}{
				"voter":    "carol",
				"author":   "alice",
				"permlink": "a-post",
				"weight":   10000,
			},
			// Order follows the whitelist scan order, not the op field order.
			want: []string{"alice", "carol"},
		},
		{
			name: "custom_json collects auth arrays",
			op: map[string]interface{}{
				"required_auths":         []interface{}{},
				"required_posting_auths": []interface{}{"dave", "erin"},
				"id":                     "follow",
			},
			want: []string{"dave", "erin"},
		},
		{
			name: "non-string owner authority is skipped",
			op: map[string]interface{}{
				"account": "frank",
				"owner": map[string]interface{}{
					"weight_threshold": 1,
					"account_auths":    []interface{}{},
				},
			},
			want: []string{"frank"},
		},
		{
			name: "curation_reward has curator and comment_author",
			op: map[string]interface{}{
				"curator":        "grace",
				"reward":         "1.000 STEEM",
				"comment_author": "alice",
			},
			want: []string{"grace", "alice"},
		},
		{
			name: "non-account scalar fields are ignored",
			op: map[string]interface{}{
				"publisher": "henry",
				"exchange_rate": map[string]interface{}{
					"base":  "1.000 SBD",
					"quote": "1.000 STEEM",
				},
			},
			want: []string{"henry"},
		},
		{
			name: "delegate_vesting_shares has delegator and delegatee",
			op: map[string]interface{}{
				"delegator":      "helen",
				"delegatee":      "ivan",
				"vesting_shares": "100.000000 VESTS",
			},
			want: []string{"helen", "ivan"},
		},
		{
			name: "fill_order has current_owner and open_owner",
			op: map[string]interface{}{
				"current_owner":   "judy",
				"open_owner":      "kyle",
				"current_orderid": 7,
			},
			want: []string{"judy", "kyle"},
		},
		{
			name: "pow2 appbase map form extracts nested worker",
			op: map[string]interface{}{
				"work": map[string]interface{}{
					"type": "pow2",
					"value": map[string]interface{}{
						"input": map[string]interface{}{"worker_account": "miner1"},
					},
				},
			},
			want: []string{"miner1"},
		},
		{
			name: "pow2 condenser list form extracts nested worker",
			op: map[string]interface{}{
				"work": []interface{}{
					map[string]interface{}{},
					map[string]interface{}{
						"input": map[string]interface{}{"worker_account": "miner2"},
					},
				},
			},
			want: []string{"miner2"},
		},
		{
			name: "nil op_value returns nil",
			op:   nil,
			want: nil,
		},
		{
			name: "empty strings are skipped",
			op: map[string]interface{}{
				"from": "",
				"to":   "ivan",
			},
			want: []string{"ivan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAccounts(tt.op)
			if len(tt.want) == 0 && len(got) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractAccounts() = %v, want %v", got, tt.want)
			}
		})
	}
}
