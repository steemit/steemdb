package refresher

import (
	"testing"
	"time"
)

func TestConvertWitness(t *testing.T) {
	raw := map[string]interface{}{
		"owner":                    "ety001",
		"votes":                    "109032233993434",
		"virtual_last_update":      "2082511560978721568",
		"virtual_position":         "4611686018427387904",
		"virtual_scheduled_time":   "340282366920938463463374607431768211455",
		"total_missed":             float64(3),
		"last_sbd_exchange_update": "2026-08-15T03:00:00",
		"running_version":          "0.23.1",
	}

	doc := convertWitness(raw)

	if v, ok := doc["votes"].(float64); !ok || v != 109032233993434 {
		t.Errorf("votes = %v (%T), want float64 109032233993434", doc["votes"], doc["votes"])
	}
	if _, ok := doc["virtual_position"].(float64); !ok {
		t.Errorf("virtual_position not converted to float64: %T", doc["virtual_position"])
	}
	if _, ok := doc["virtual_scheduled_time"].(float64); !ok {
		t.Errorf("virtual_scheduled_time not converted to float64: %T", doc["virtual_scheduled_time"])
	}
	ts, ok := doc["last_sbd_exchange_update"].(time.Time)
	if !ok {
		t.Fatalf("last_sbd_exchange_update not converted to time.Time: %T", doc["last_sbd_exchange_update"])
	}
	if ts.Format(steemDateFormat) != "2026-08-15T03:00:00" {
		t.Errorf("last_sbd_exchange_update = %v, want 2026-08-15T03:00:00", ts)
	}
	// Untouched fields pass through
	if doc["owner"] != "ety001" || doc["running_version"] != "0.23.1" {
		t.Errorf("passthrough fields changed: owner=%v running_version=%v", doc["owner"], doc["running_version"])
	}
	// Input map is not mutated
	if raw["votes"] != "109032233993434" {
		t.Errorf("input map mutated: votes=%v", raw["votes"])
	}
}

func TestDetectMiss(t *testing.T) {
	w := &witnessRefresher{misses: make(map[string]int64)} // only the misses map is exercised
	now := time.Now()

	// First observation: baseline only, no record
	if rec := w.detectMiss("alice", 10, now); rec != nil {
		t.Errorf("first observation produced record: %v", rec)
	}
	// No change
	if rec := w.detectMiss("alice", 10, now); rec != nil {
		t.Errorf("unchanged total produced record: %v", rec)
	}
	// Increase: record with delta
	rec := w.detectMiss("alice", 13, now)
	if rec == nil {
		t.Fatal("increase produced no record")
	}
	if rec["increase"] != int64(3) || rec["total"] != int64(13) || rec["witness"] != "alice" {
		t.Errorf("record fields wrong: %v", rec)
	}
	// Decrease (witness restarted chain data): baseline only
	if rec := w.detectMiss("alice", 2, now); rec != nil {
		t.Errorf("decrease produced record: %v", rec)
	}
	// Missing field sentinel (-1): no record
	if rec := w.detectMiss("bob", -1, now); rec != nil {
		t.Errorf("missing total_missed produced record: %v", rec)
	}
}

func TestReadInt64(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int64
	}{
		{float64(42), 42},
		{"123", 123},
		{"not-a-number", -1},
		{nil, -1},
	}
	for _, c := range cases {
		if got := readInt64(map[string]interface{}{"k": c.in}, "k"); got != c.want {
			t.Errorf("readInt64(%v) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := readInt64(map[string]interface{}{}, "missing"); got != -1 {
		t.Errorf("readInt64(missing) = %d, want -1", got)
	}
}

func TestConvertRewardFund(t *testing.T) {
	raw := map[string]interface{}{
		"id":                      float64(0),
		"name":                    "post",
		"recent_claims":           "2168005707985724",
		"content_constant":        "2000000000000",
		"reward_balance":          "157688.348 STEEM",
		"percent_content_rewards": float64(10000),
		"last_update":             "2026-08-15T10:00:00",
	}

	doc := convertRewardFund(raw)

	if v, ok := doc["recent_claims"].(float64); !ok || v != 2168005707985724 {
		t.Errorf("recent_claims = %v (%T)", doc["recent_claims"], doc["recent_claims"])
	}
	if v, ok := doc["reward_balance"].(float64); !ok || v != 157688.348 {
		t.Errorf("reward_balance = %v (%T)", doc["reward_balance"], doc["reward_balance"])
	}
	if _, ok := doc["last_update"].(time.Time); !ok {
		t.Errorf("last_update not converted: %T", doc["last_update"])
	}
	if doc["name"] != "post" {
		t.Errorf("name changed: %v", doc["name"])
	}
}
