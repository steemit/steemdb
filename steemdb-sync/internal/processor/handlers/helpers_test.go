package handlers

import (
	"testing"
)

func TestParseAsset_StringFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantVal float64
		wantSym string
	}{
		{"STEEM amount", "1.000 STEEM", 1.0, "STEEM"},
		{"SBD amount", "2.500 SBD", 2.5, "SBD"},
		{"VESTS amount", "1234.567890 VESTS", 1234.567890, "VESTS"},
		{"zero amount", "0.000 STEEM", 0.0, "STEEM"},
		{"large amount", "1000000.000 STEEM", 1000000.0, "STEEM"},
		{"empty string", "", 0, ""},
		{"no unit", "1.000", 0, ""},
		{"invalid number", "abc STEEM", 0, "STEEM"},
		{"negative amount", "-5.000 SBD", -5.0, "SBD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotSym := ParseAsset(tt.input)
			if gotVal != tt.wantVal {
				t.Errorf("ParseAsset(%q) value = %v, want %v", tt.input, gotVal, tt.wantVal)
			}
			if gotSym != tt.wantSym {
				t.Errorf("ParseAsset(%q) symbol = %q, want %q", tt.input, gotSym, tt.wantSym)
			}
		})
	}
}

func TestParseAsset_NAIFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		wantVal float64
		wantSym string
	}{
		{
			"STEEM NAI",
			map[string]interface{}{"nai": "@@000000021", "amount": "833000", "precision": float64(3)},
			833.0, "STEEM",
		},
		{
			"SBD NAI",
			map[string]interface{}{"nai": "@@000000013", "amount": "2500000", "precision": float64(3)},
			2500.0, "SBD",
		},
		{
			"VESTS NAI",
			map[string]interface{}{"nai": "@@000000037", "amount": "1234567890", "precision": float64(6)},
			1234.567890, "VESTS",
		},
		{
			"zero STEEM NAI",
			map[string]interface{}{"nai": "@@000000021", "amount": "0", "precision": float64(3)},
			0.0, "STEEM",
		},
		{
			"unknown NAI",
			map[string]interface{}{"nai": "@@999999999", "amount": "1000", "precision": float64(3)},
			1.0, "", // value parsed, symbol unknown
		},
		{
			"missing amount",
			map[string]interface{}{"nai": "@@000000021", "precision": float64(3)},
			0.0, "STEEM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotSym := ParseAsset(tt.input)
			if gotVal != tt.wantVal {
				t.Errorf("ParseAsset(NAI) value = %v, want %v", gotVal, tt.wantVal)
			}
			if gotSym != tt.wantSym {
				t.Errorf("ParseAsset(NAI) symbol = %q, want %q", gotSym, tt.wantSym)
			}
		})
	}
}

func TestParseAsset_NilAndInvalid(t *testing.T) {
	// nil
	val, sym := ParseAsset(nil)
	if val != 0 || sym != "" {
		t.Errorf("ParseAsset(nil) = (%v, %q), want (0, \"\")", val, sym)
	}

	// invalid type (int)
	val, sym = ParseAsset(42)
	if val != 0 || sym != "" {
		t.Errorf("ParseAsset(42) = (%v, %q), want (0, \"\")", val, sym)
	}
}

func TestAssetValue(t *testing.T) {
	// String format
	if v := AssetValue("1.000 STEEM"); v != 1.0 {
		t.Errorf("AssetValue(string) = %v, want 1.0", v)
	}
	// NAI format
	nai := map[string]interface{}{"nai": "@@000000021", "amount": "833000", "precision": float64(3)}
	if v := AssetValue(nai); v != 833.0 {
		t.Errorf("AssetValue(NAI) = %v, want 833.0", v)
	}
	// nil
	if v := AssetValue(nil); v != 0 {
		t.Errorf("AssetValue(nil) = %v, want 0", v)
	}
}

func TestAssetSymbol(t *testing.T) {
	// String format
	if s := AssetSymbol("1.000 STEEM"); s != "STEEM" {
		t.Errorf("AssetSymbol(string) = %q, want STEEM", s)
	}
	// NAI format
	nai := map[string]interface{}{"nai": "@@000000013", "amount": "100", "precision": float64(3)}
	if s := AssetSymbol(nai); s != "SBD" {
		t.Errorf("AssetSymbol(NAI) = %q, want SBD", s)
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want string
	}{
		{"existing string", map[string]interface{}{"name": "alice"}, "name", "alice"},
		{"missing key", map[string]interface{}{"name": "alice"}, "other", ""},
		{"nil map", nil, "name", ""},
		{"non-string value", map[string]interface{}{"count": 42}, "count", ""},
		{"empty string value", map[string]interface{}{"name": ""}, "name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetString(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("GetString(%v, %q) = %q, want %q", tt.m, tt.key, got, tt.want)
			}
		})
	}
}

func TestGetField(t *testing.T) {
	m := map[string]interface{}{"name": "alice", "amount": map[string]interface{}{"nai": "@@000000021"}}
	if v := GetField(m, "name"); v != "alice" {
		t.Errorf("GetField(name) = %v, want alice", v)
	}
	if v := GetField(m, "amount"); v == nil {
		t.Error("GetField(amount) should not be nil")
	}
	if v := GetField(m, "missing"); v != nil {
		t.Errorf("GetField(missing) = %v, want nil", v)
	}
}
