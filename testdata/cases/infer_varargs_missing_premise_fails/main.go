// Variadic From requires EVERY premise to discharge on the same
// variable. Here the caller establishes isEven but not
// isSmallPositive, so the rule's premise (isEven AND
// isSmallPositive) is not fully discharged and target's isPositive
// obligation remains unproven — the build must fail.

package main

import (
	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool      { return x > 0 }
func isEven(x int) bool          { return x%2 == 0 }
func isSmallPositive(x int) bool { return x > 0 && x < 100 }

var _ = infer.From(isEven, isSmallPositive).To(isPositive)

func target(x int) {
	proven.That(x, isPositive)
}

func main() {
	x := 4
	if isEven(x) {
		target(x) // isSmallPositive missing — rule can't fire
	}
}
