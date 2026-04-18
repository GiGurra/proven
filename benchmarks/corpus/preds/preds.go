// Package preds is the corpus's central library of predicates.
// Real codebases typically have a smaller set of "core" predicates
// imported by many leaf packages; this package simulates that.
package preds

import "strings"

// Integer predicates.
func IsPositive(x int) bool      { return x > 0 }
func IsNegative(x int) bool      { return x < 0 }
func IsNonNeg(x int) bool        { return x >= 0 }
func IsEven(x int) bool          { return x%2 == 0 }
func IsOdd(x int) bool           { return x%2 != 0 }
func IsSmall(x int) bool         { return x >= 0 && x < 100 }
func IsLarge(x int) bool         { return x >= 1000 }
func IsSmallPositive(x int) bool { return x > 0 && x < 100 }
func IsMidRange(x int) bool      { return x >= 100 && x < 1000 }
func IsInByteRange(x int) bool   { return x >= 0 && x < 256 }

// String predicates.
func IsNonEmpty(s string) bool  { return len(s) > 0 }
func IsAllLower(s string) bool  { return s == strings.ToLower(s) }
func IsAllUpper(s string) bool  { return s == strings.ToUpper(s) }
func HasPrefix_(s string) bool  { return strings.HasPrefix(s, "_") }
func NoWhitespace(s string) bool {
	return !strings.ContainsAny(s, " \t\n\r")
}

// Slice predicates.
func IsNonEmptyInts(xs []int) bool { return len(xs) > 0 }
func IsSortedInts(xs []int) bool {
	for i := 1; i < len(xs); i++ {
		if xs[i-1] > xs[i] {
			return false
		}
	}
	return true
}
