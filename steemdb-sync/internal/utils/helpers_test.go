package utils

import (
	"testing"
)

func TestParseAmountValue(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"10.000 STEEM", 10.0},
		{"5.123 SBD", 5.123},
		{"0.001 VESTS", 0.001},
		{"", 0.0},
		{"invalid", 0.0},
		{"10", 10.0}, // Just number, should parse the number part
	}

	for _, test := range tests {
		result := ParseAmountValue(test.input)
		if result != test.expected {
			t.Errorf("ParseAmountValue(%q) = %f, expected %f", test.input, result, test.expected)
		}
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		input           string
		expectedAmount  float64
		expectedCurrency string
	}{
		{"10.000 STEEM", 10.0, "STEEM"},
		{"5.123 SBD", 5.123, "SBD"},
		{"0.001 VESTS", 0.001, "VESTS"},
		{"", 0.0, ""},
		{"invalid", 0.0, ""},
		{"10", 0.0, ""}, // Missing currency
	}

	for _, test := range tests {
		amount, currency := ParseAmount(test.input)
		if amount != test.expectedAmount || currency != test.expectedCurrency {
			t.Errorf("ParseAmount(%q) = (%f, %q), expected (%f, %q)", 
				test.input, amount, currency, test.expectedAmount, test.expectedCurrency)
		}
	}
}

func TestParseFloat64Value(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"123.456", 123.456},
		{"0", 0.0},
		{"-10.5", -10.5},
		{"", 0.0},
		{"invalid", 0.0},
		{"123.456.789", 0.0}, // Invalid format
	}

	for _, test := range tests {
		result := ParseFloat64Value(test.input)
		if result != test.expected {
			t.Errorf("ParseFloat64Value(%q) = %f, expected %f", test.input, result, test.expected)
		}
	}
}
