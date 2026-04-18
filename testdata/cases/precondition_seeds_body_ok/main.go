// A function's own declared precondition seeds the analyzer's
// fact set inside that function's body. If F declares
// proven.That(x, isPositive) at the top, then inside F's body
// isPositive(x) is a starting fact — exactly what every caller
// has proved at the call site. F can then call G(x) where G also
// requires isPositive without an intervening guard, because the
// fact is already in scope.
//
// This locks in the fact-set seeding behavior that makes
// proven.Returns verification practical (returns_via_precondition_ok
// covers the same pattern for Returns).

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func G(x int) {
	proven.That(x, isPositive)
}

func F(x int) {
	proven.That(x, isPositive) // precondition seeds fact(isPositive, x)
	G(x)                       // discharged without a re-guard
}

func main() {
	x := 5
	if isPositive(x) {
		F(x)
	}
}
