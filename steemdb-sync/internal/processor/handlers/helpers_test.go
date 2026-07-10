package handlers

import (
	"testing"
)

func TestSplitAmount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVal  float64
		wantUnit string
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
			gotVal, gotUnit := SplitAmount(tt.input)
			if gotVal != tt.wantVal {
				t.Errorf("SplitAmount(%q) value = %v, want %v", tt.input, gotVal, tt.wantVal)
			}
			if gotUnit != tt.wantUnit {
				t.Errorf("SplitAmount(%q) unit = %q, want %q", tt.input, gotUnit, tt.wantUnit)
			}
		})
	}
}

func TestAmountValue(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1.000 STEEM", 1.0},
		{"99.999 SBD", 99.999},
		{"", 0},
	}
	for _, tt := range tests {
		got := AmountValue(tt.input)
		if got != tt.want {
			t.Errorf("AmountValue(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAmountUnit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.000 STEEM", "STEEM"},
		{"99.999 SBD", "SBD"},
		{"", ""},
	}
	for _, tt := range tests {
		got := AmountUnit(tt.input)
		if got != tt.want {
			t.Errorf("AmountUnit(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
