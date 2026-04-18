// Variadic From: the rule's premise AND-composes. Two separate
// facts (isEven AND isSmallPositive) both established on x via
// preceding guards let the preprocessor discharge the isPositive
// obligation at target via backward-chaining through the rule.

package main

import (
	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool      { return x > 0 }
func isEven(x int) bool          { return x%2 == 0 }
func isSmallPositive(x int) bool { return x > 0 && x < 100 }

// isEven(x) AND isSmallPositive(x) implies isPositive(x).
var _ = infer.From(isEven, isSmallPositive).To(isPositive)

func target(x int) {
	proven.That(x, isPositive)
}

func main() {
	x := 4
	if isEven(x) {
		if isSmallPositive(x) {
			target(x)
		}
	}
}
