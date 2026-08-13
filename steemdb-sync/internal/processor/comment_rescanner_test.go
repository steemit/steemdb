package processor

import (
	"encoding/json"
	"testing"
	"time"

	protocolapi "github.com/steemit/steemutil/protocol/api"
)

func TestProcessContent_BasicTransforms(t *testing.T) {
	content := &protocolapi.Content{
		Author:                  "Alice",
		Permlink:                "My-Post",
		Category:                "Test",
		Depth:                   0,
		NetVotes:                5,
		PendingPayoutValue:      "1.234 SBD",
		TotalPayoutValue:        "0.000 SBD",
		CashoutTime:             "2024-01-15T10:30:00",
		Created:                 "2024-01-08T10:30:00",
		AuthorReputation:        json.RawMessage(`"12345678901"`),
		JSONMetadata:            `{"tags":["test"]}`,
		ActiveVotes: []protocolapi.VoteState{
			{
				Voter:   "bob",
				Rshares: json.RawMessage(`"1000000"`),
				Weight:  json.RawMessage(`"5000"`),
				Time:    "2024-01-08T11:00:00",
			},
		},
	}

	doc := processContent(content)

	// Check asset transforms
	if doc["pending_payout_value"] != 1.234 {
		t.Errorf("pending_payout_value = %v, want 1.234", doc["pending_payout_value"])
	}

	// Check date transforms
	if _, ok := doc["cashout_time"].(time.Time); !ok {
		t.Errorf("cashout_time should be time.Time, got %T", doc["cashout_time"])
	}

	// Check json_metadata parsed
	if _, ok := doc["json_metadata"].(map[string]interface{}); !ok {
		t.Errorf("json_metadata should be map, got %T", doc["json_metadata"])
	}

	// Check derived fields
	if doc["author_lower"] != "alice" {
		t.Errorf("author_lower = %v, want alice", doc["author_lower"])
	}
	if doc["category_lower"] != "test" {
		t.Errorf("category_lower = %v, want test", doc["category_lower"])
	}
	if doc["date_idx"] != "2024-01-08" {
		t.Errorf("date_idx = %v, want 2024-01-08", doc["date_idx"])
	}

	// Check scanned is set
	if _, ok := doc["scanned"].(time.Time); !ok {
		t.Errorf("scanned should be time.Time, got %T", doc["scanned"])
	}
}

func TestProcessContent_ActiveVotesTransform(t *testing.T) {
	content := &protocolapi.Content{
		ActiveVotes: []protocolapi.VoteState{
			{
				Voter:   "alice",
				Rshares: json.RawMessage(`"5000000"`),
				Weight:  json.RawMessage(`"10000"`),
				Time:    "2024-01-08T12:00:00",
			},
			{
				Voter:   "bob",
				Rshares: json.RawMessage(`"3000000"`),
				Weight:  json.RawMessage(`"8000"`),
				Time:    "2024-01-08T12:01:00",
			},
		},
	}

	doc := processContent(content)

	votes, ok := doc["active_votes"].([]interface{})
	if !ok {
		t.Fatalf("active_votes should be []interface{}, got %T", doc["active_votes"])
	}
	if len(votes) != 2 {
		t.Fatalf("expected 2 votes, got %d", len(votes))
	}

	// Check first vote's rshares transformed to float
	vote0 := votes[0].(map[string]interface{})
	if vote0["rshares"] != 5000000.0 {
		t.Errorf("vote[0] rshares = %v, want 5000000.0", vote0["rshares"])
	}

	vote1 := votes[1].(map[string]interface{})
	if vote1["weight"] != 8000.0 {
		t.Errorf("vote[1] weight = %v, want 8000.0", vote1["weight"])
	}

	// time should be parsed
	if _, ok := vote0["time"].(time.Time); !ok {
		t.Errorf("vote[0] time should be time.Time, got %T", vote0["time"])
	}
}

func TestProcessContent_BadJsonMetadata(t *testing.T) {
	content := &protocolapi.Content{
		JSONMetadata: "not valid json {{{",
	}

	doc := processContent(content)

	// Should keep raw string on parse failure
	if doc["json_metadata"] != "not valid json {{{" {
		t.Errorf("bad json_metadata should keep raw string, got %v", doc["json_metadata"])
	}
}

func TestProcessContent_EmptyJsonMetadata(t *testing.T) {
	content := &protocolapi.Content{
		JSONMetadata: "",
	}

	doc := processContent(content)

	// Empty string should stay empty
	if doc["json_metadata"] != "" {
		t.Errorf("empty json_metadata should stay empty, got %v", doc["json_metadata"])
	}
}

func TestDeduplicateComments(t *testing.T) {
	refs := []commentRef{
		{Author: "alice", Permlink: "post1"},
		{Author: "bob", Permlink: "post2"},
		{Author: "alice", Permlink: "post1"}, // duplicate
		{Author: "charlie", Permlink: "post3"},
		{Author: "bob", Permlink: "post2"}, // duplicate
	}

	result := deduplicateComments(refs)

	if len(result) != 3 {
		t.Errorf("expected 3 unique refs, got %d", len(result))
	}
}

func TestTransformRawMsgToFloat(t *testing.T) {
	m := map[string]interface{}{
		"string_num": "12345",
		"already":    42.5,
		"raw_msg":    json.RawMessage("67890"),
	}

	transformRawMsgToFloat(m, "string_num")
	transformRawMsgToFloat(m, "already")
	transformRawMsgToFloat(m, "raw_msg")
	transformRawMsgToFloat(m, "missing")

	if m["string_num"] != 12345.0 {
		t.Errorf("string_num = %v, want 12345.0", m["string_num"])
	}
	if m["already"] != 42.5 {
		t.Errorf("already = %v, want 42.5", m["already"])
	}
	if m["raw_msg"] != 67890.0 {
		t.Errorf("raw_msg = %v, want 67890.0", m["raw_msg"])
	}
}
