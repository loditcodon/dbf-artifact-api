package utils

import (
	"fmt"
	"math"
	"strings"
)

// SafeIntToUint safely converts int to uint with validation.
// Returns error if value is negative.
// Use this when converting database model fields (int) to GORM ID fields (uint).
func SafeIntToUint(val int) (uint, error) {
	if val < 0 {
		return 0, fmt.Errorf("cannot convert negative int %d to uint", val)
	}
	return uint(val), nil
}

// SafeUintToInt safely converts uint to int with overflow check.
// Returns error if value exceeds max int value.
// Use this when converting GORM ID fields (uint) to database model fields (int).
func SafeUintToInt(val uint) (int, error) {
	if val > math.MaxInt {
		return 0, fmt.Errorf("uint value %d exceeds maximum int value", val)
	}
	return int(val), nil
}

// MustIntToUint converts int to uint, panics on negative values.
// Only use in contexts where negative values are impossible.
func MustIntToUint(val int) uint {
	if val < 0 {
		panic(fmt.Sprintf("unexpected negative value %d in MustIntToUint", val))
	}
	return uint(val)
}

// MustUintToInt converts uint to int, panics on overflow.
// Only use in contexts where overflow is impossible.
func MustUintToInt(val uint) int {
	if val > math.MaxInt {
		panic(fmt.Sprintf("unexpected overflow in MustUintToInt: %d", val))
	}
	return int(val)
}

// IntToUintOrZero converts int to uint, returns 0 if negative.
// Use when negative values should be treated as zero/absent.
func IntToUintOrZero(val int) uint {
	if val < 0 {
		return 0
	}
	return uint(val)
}

// EscapeSQL escapes single quotes in SQL strings to prevent injection.
// Used by privilege session code for building INSERT statements into in-memory MySQL server.
func EscapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
