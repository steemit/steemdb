package processor

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProcessContent_BasicTransforms(t *testing.T) {
	content := map[string]interface{}{
		"author":                 "Alice",
		"permlink":               "My-Post",
		"category":               "Test",
		"depth":                  0,
		"net_votes":              5,
		"pending_payout_value":   "1.234 SBD",
		"total_payout_value":     "0.000 SBD",
		"cashout_time":           "2024-01-15T10:30:00",
		"created":                "2024-01-08T10:30:00",
		"author_reputation":      "12345678901",
		"json_metadata":          `{"tags":["test"]}`,
		"active_votes": []interface{}{
			map[string]interface{}{
				"voter":   "bob",
				"rshares": "1000000",
				"weight":  "5000",
				"time":    "2024-01-08T11:00:00",
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

	// Check float transform
	if doc["author_reputation"] != 12345678901.0 {
		t.Errorf("author_reputation = %v, want 12345678901.0", doc["author_reputation"])
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
	content := map[string]interface{}{
		"active_votes": []interface{}{
			map[string]interface{}{
				"voter":   "alice",
				"rshares": "5000000",
				"weight":  "10000",
				"time":    "2024-01-08T12:00:00",
			},
			map[string]interface{}{
				"voter":   "bob",
				"rshares": "3000000",
				"weight":  "8000",
				"time":    "2024-01-08T12:01:00",
			},
		},
	}

	doc := processContent(content)

	votes, ok := doc["active_votes"].([]map[string]interface{})
	if !ok {
		t.Fatalf("active_votes should be []map, got %T", doc["active_votes"])
	}
	if len(votes) != 2 {
		t.Fatalf("expected 2 votes, got %d", len(votes))
	}

	// rshares should be float
	if votes[0]["rshares"] != 5000000.0 {
		t.Errorf("vote[0] rshares = %v, want 5000000.0", votes[0]["rshares"])
	}
	if votes[1]["weight"] != 8000.0 {
		t.Errorf("vote[1] weight = %v, want 8000.0", votes[1]["weight"])
	}

	// time should be parsed
	if _, ok := votes[0]["time"].(time.Time); !ok {
		t.Errorf("vote[0] time should be time.Time, got %T", votes[0]["time"])
	}
}

func TestProcessContent_BadJsonMetadata(t *testing.T) {
	content := map[string]interface{}{
		"json_metadata": "not valid json {{{",
	}

	doc := processContent(content)

	// Should keep raw string on parse failure
	if doc["json_metadata"] != "not valid json {{{" {
		t.Errorf("bad json_metadata should keep raw string, got %v", doc["json_metadata"])
	}
}

func TestProcessContent_EmptyJsonMetadata(t *testing.T) {
	content := map[string]interface{}{
		"json_metadata": "",
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
