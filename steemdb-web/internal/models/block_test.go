package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBlockSummaryJSONSerialization(t *testing.T) {
	summary := BlockSummary{
		Number:           12345,
		BlockNum:         12345,
		Timestamp:        time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC),
		Witness:          "steem",
		TransactionCount: 3,
		OperationCount:   5,
		Previous:         "00003038...",
	}

	bytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal BlockSummary: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if num, ok := data["number"].(float64); !ok || uint32(num) != 12345 {
		t.Errorf("expected number=12345, got %v", data["number"])
	}

	if bnum, ok := data["block_num"].(float64); !ok || uint32(bnum) != 12345 {
		t.Errorf("expected block_num=12345, got %v", data["block_num"])
	}

	if witness, ok := data["witness"].(string); !ok || witness != "steem" {
		t.Errorf("expected witness='steem', got %v", data["witness"])
	}
}
