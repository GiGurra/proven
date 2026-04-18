// Variadic To: one premise fans out into multiple conclusions.
// isSmallPositive implies BOTH isPositive AND isNonNeg, so one
// guard in the caller discharges two separate target obligations.

package main

import (
	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool      { return x > 0 }
func isNonNeg(x int) bool        { return x >= 0 }
func isSmallPositive(x int) bool { return x > 0 && x < 100 }

var _ = infer.From(isSmallPositive).To(isPositive, isNonNeg)

func needsPositive(x int) { proven.That(x, isPositive) }
func needsNonNeg(x int)   { proven.That(x, isNonNeg) }

func main() {
	x := 7
	if isSmallPositive(x) {
		needsPositive(x)
		needsNonNeg(x)
	}
}
