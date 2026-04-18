// proven.Returns(v, proven.And(a, b)) advertises both leaves as the
// function's postcondition and verifies each leaf on v at the
// declaration site. Here the returned value carries both leaves as
// preconditions (seeded via proven.That on the parameter before the
// return), so the strict verifier sees every leaf as already
// discharged.

package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func pickPositiveSmall(p int) int {
	// Seed both leaves as preconditions on p, so the returned value
	// — which is p — satisfies both advertised postconditions.
	proven.That(p, proven.And(isPositive, lessThan100))
	return proven.Returns(p, proven.And(isPositive, lessThan100))
}

func target(x int) {
	proven.That(x, isPositive, lessThan100)
}

func main() {
	seed := 42
	if isPositive(seed) && lessThan100(seed) {
		v := pickPositiveSmall(seed)
		target(v)
		fmt.Println(v)
	}
}
