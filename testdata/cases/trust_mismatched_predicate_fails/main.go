// trust.That only injects the predicates it is given; it is not
// a universal override. Asserting one predicate and then calling
// a target that requires a different predicate still fails the
// build — trust is local fact injection, not a free discharge of
// every obligation.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }
func isEven(x int) bool     { return x%2 == 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	raw := 4
	// We trust-inject isEven, but target needs isPositive.
	// The analyzer should leave isPositive undischarged.
	v := trust.That(raw, isEven)
	target(v)
}
