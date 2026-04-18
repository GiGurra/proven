// Package basic demonstrates the proven design: plain Go signatures,
// with proven.That declaring preconditions in the body and proven.Returns
// declaring postconditions on return values. Predicates are ordinary
// Go functions.
package basic

import "github.com/GiGurra/proven/pkg/proven"

// Predicates: plain Go, nothing special.

func isPositive(x int) bool    { return x > 0 }
func lessThan100(x int) bool   { return x < 100 }
func isNonEmpty(s string) bool { return len(s) > 0 }
func maxLen280(s string) bool  { return len(s) <= 280 }

func validCurrency(s string) bool {
	switch s {
	case "USD", "EUR", "GBP":
		return true
	}
	return false
}

// Transfer's signature is pure Go. Preconditions are declared at the
// top of the body via proven.That. Under the preprocessor, every call
// site must prove these predicates for the arguments it passes; the
// That calls are then erased. Without the preprocessor, That is a
// runtime contract check that panics on violation.
func Transfer(amount int, note string, currency string) error {
	proven.That(amount, isPositive)
	proven.That(note, isNonEmpty, maxLen280)
	proven.That(currency, validCurrency)
	_ = amount
	_ = note
	_ = currency
	return nil
}

// SetPercent requires p in (0, 100). Variadic That AND-composes.
func SetPercent(p int) {
	proven.That(p, isPositive, lessThan100)
	_ = p
}

// FindUserID returns a positive user ID. Returns declares the
// postcondition; callers can use the returned value wherever isPositive
// is required without re-proving.
func FindUserID(name string) int {
	_ = name
	return proven.Returns(42, isPositive)
}
