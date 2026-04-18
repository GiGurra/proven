package infer_test

import (
	"testing"

	"github.com/GiGurra/proven/pkg/infer"
)

func isPositive(x int) bool        { return x > 0 }
func isSmallPositive(x int) bool   { return x > 0 && x < 100 }
func isEven(x int) bool            { return x%2 == 0 }
func isGreaterThanZero(x int) bool { return x > 0 }

// Package-scope inference declarations. These compile-time facts are
// what the proven preprocessor will scan for and add to its implication
// graph when it is built.

var _ = infer.From(isSmallPositive).To(isPositive)
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

// TestBuilderCompiles exists only to ensure the fluent builder produces
// valid Rule values. There is no runtime behavior to exercise yet — the
// preprocessor consumes these declarations.
func TestBuilderCompiles(t *testing.T) {
	r1 := infer.From(isSmallPositive).To(isPositive)
	r2 := infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
	_ = r1
	_ = r2
}
