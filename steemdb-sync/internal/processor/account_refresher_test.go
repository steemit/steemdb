package processor

import (
	"testing"
	"time"
)

func TestAssetValueFromString(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"100.000 STEEM", 100.0},
		{"0.500 SBD", 0.5},
		{"1234.567890 VESTS", 1234.567890},
		{"0.000 STEEM", 0.0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		got := assetValueFromString(tt.input)
		if got != tt.want {
			t.Errorf("assetValueFromString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTransformAssetValue(t *testing.T) {
	m := map[string]interface{}{
		"balance":         "100.000 STEEM",
		"vesting_shares":  "5000.000000 VESTS",
		"sbd_balance":     "0.000 SBD",
		"already_float":   42.5,
		"not_string":      123,
	}

	transformAssetValue(m, "balance")
	transformAssetValue(m, "vesting_shares")
	transformAssetValue(m, "sbd_balance")
	transformAssetValue(m, "already_float")
	transformAssetValue(m, "not_string")

	if m["balance"] != 100.0 {
		t.Errorf("balance = %v, want 100.0", m["balance"])
	}
	if m["vesting_shares"] != 5000.0 {
		t.Errorf("vesting_shares = %v, want 5000.0", m["vesting_shares"])
	}
	if m["sbd_balance"] != 0.0 {
		t.Errorf("sbd_balance = %v, want 0.0", m["sbd_balance"])
	}
	// already float should stay
	if m["already_float"] != 42.5 {
		t.Errorf("already_float = %v, want 42.5", m["already_float"])
	}
	// non-string should be untouched
	if m["not_string"] != 123 {
		t.Errorf("not_string = %v, want 123", m["not_string"])
	}
}

func TestTransformDate(t *testing.T) {
	m := map[string]interface{}{
		"created":    "2016-03-24T16:05:00",
		"empty":      "",
		"not_string": 12345,
		"bad_format": "not-a-date",
	}

	transformDate(m, "created")
	transformDate(m, "empty")
	transformDate(m, "not_string")
	transformDate(m, "bad_format")

	if tm, ok := m["created"].(time.Time); !ok {
		t.Errorf("created should be time.Time, got %T", m["created"])
	} else {
		expected := time.Date(2016, 3, 24, 16, 5, 0, 0, time.UTC)
		if !tm.Equal(expected) {
			t.Errorf("created = %v, want %v", tm, expected)
		}
	}

	// Empty string should not be transformed (stays as "")
	if m["empty"] != "" {
		t.Errorf("empty date should stay empty, got %v", m["empty"])
	}
	// Bad format should stay as string
	if m["bad_format"] != "not-a-date" {
		t.Errorf("bad_format should stay as string, got %v", m["bad_format"])
	}
}

func TestProcessAccount_Transforms(t *testing.T) {
	// Use a map[string]interface{} as the "account" — processAccount marshals to JSON
	// then unmarshals, so we can test with any struct/map that serializes correctly.
	acct := map[string]interface{}{
		"name":               "alice",
		"balance":            "100.000 STEEM",
		"sbd_balance":        "5.000 SBD",
		"savings_balance":    "50.000 STEEM",
		"savings_sbd_balance": "1.000 SBD",
		"vesting_shares":     "10000.000000 VESTS",
		"reputation":         "12345678",
		"created":            "2016-03-24T16:05:00",
		"last_post":          "2024-01-15T10:30:00",
		"proxied_vsf_votes":  []interface{}{float64(5000000)},
	}

	doc := processAccount(acct)

	// Check asset transforms
	if doc["balance"] != 100.0 {
		t.Errorf("balance = %v, want 100.0", doc["balance"])
	}
	if doc["vesting_shares"] != 10000.0 {
		t.Errorf("vesting_shares = %v, want 10000.0", doc["vesting_shares"])
	}

	// Check computed fields
	if doc["total_balance"] != 150.0 {
		t.Errorf("total_balance = %v, want 150.0 (100+50)", doc["total_balance"])
	}
	if doc["total_sbd_balance"] != 6.0 {
		t.Errorf("total_sbd_balance = %v, want 6.0 (5+1)", doc["total_sbd_balance"])
	}

	// Check proxy_witness = proxied_vsf_votes[0] / 1000000
	if doc["proxy_witness"] != 5.0 {
		t.Errorf("proxy_witness = %v, want 5.0", doc["proxy_witness"])
	}

	// Check date transform
	if _, ok := doc["created"].(time.Time); !ok {
		t.Errorf("created should be time.Time, got %T", doc["created"])
	}

	// Check scanned is set
	if _, ok := doc["scanned"].(time.Time); !ok {
		t.Errorf("scanned should be time.Time, got %T", doc["scanned"])
	}
}

func TestProcessAccount_EmptyProxiedVSFVotes(t *testing.T) {
	acct := map[string]interface{}{
		"name":              "bob",
		"balance":           "0.000 STEEM",
		"proxied_vsf_votes": []interface{}{}, // empty array
	}

	doc := processAccount(acct)

	// proxy_witness should not be set if proxied_vsf_votes is empty
	if _, exists := doc["proxy_witness"]; exists {
		t.Errorf("proxy_witness should not be set for empty proxied_vsf_votes")
	}
}

func TestGetFloat(t *testing.T) {
	m := map[string]interface{}{
		"a": float64(42.5),
		"b": "not a number",
		"c": 123, // int
	}

	if getFloat(m, "a") != 42.5 {
		t.Errorf("getFloat(a) = %v, want 42.5", getFloat(m, "a"))
	}
	if getFloat(m, "b") != 0 {
		t.Errorf("getFloat(b) = %v, want 0", getFloat(m, "b"))
	}
	if getFloat(m, "missing") != 0 {
		t.Errorf("getFloat(missing) = %v, want 0", getFloat(m, "missing"))
	}
}

func TestParseFloatSafe(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"123.456", 123.456},
		{"0", 0},
		{"-5.5", -5.5},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseFloatSafe(tt.input)
		if got != tt.want {
			t.Errorf("parseFloatSafe(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
