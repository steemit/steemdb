package utils

import (
	"testing"
)

func BenchmarkParseAmountValue(b *testing.B) {
	testCases := []string{
		"10.000 STEEM",
		"5.123 SBD",
		"0.001 VESTS",
		"1234567.890123 STEEM",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			ParseAmountValue(tc)
		}
	}
}

func BenchmarkParseAmount(b *testing.B) {
	testCases := []string{
		"10.000 STEEM",
		"5.123 SBD", 
		"0.001 VESTS",
		"1234567.890123 STEEM",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			ParseAmount(tc)
		}
	}
}

func BenchmarkParseFloat64Value(b *testing.B) {
	testCases := []string{
		"123.456",
		"0",
		"-10.5",
		"1234567.890123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			ParseFloat64Value(tc)
		}
	}
}
