package utils

import (
	"strconv"
	"strings"
)

// ParseAmountValue parses amount string and returns the numeric value
func ParseAmountValue(amountStr string) float64 {
	parts := strings.Fields(amountStr)
	if len(parts) == 0 {
		return 0
	}
	
	if amount, err := strconv.ParseFloat(parts[0], 64); err == nil {
		return amount
	}
	return 0
}

// ParseAmount parses amount string and returns value and currency
func ParseAmount(amountStr string) (float64, string) {
	parts := strings.Fields(amountStr)
	if len(parts) != 2 {
		return 0, ""
	}

	amount, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, parts[1]
	}

	return amount, parts[1]
}

// ParseFloat64 parses string to float64
func ParseFloat64(str string) (float64, error) {
	return strconv.ParseFloat(str, 64)
}

// ParseFloat64Value parses string to float64, returns 0 on error
func ParseFloat64Value(str string) float64 {
	if val, err := ParseFloat64(str); err == nil {
		return val
	}
	return 0
}
