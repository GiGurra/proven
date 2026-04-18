// The function declares proven.That(x, isPositive) as its own
// precondition. The analyzer seeds the starting fact set with
// that precondition, so inside the body x already has isPositive
// as a fact — which is exactly what every caller has proved at
// its call site. proven.Returns(x, isPositive) verifies cleanly
// against this seeded fact.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source(x int) int {
	proven.That(x, isPositive)
	return proven.Returns(x, isPositive)
}

func target(v int) {
	proven.That(v, isPositive)
}

func main() {
	x := 42
	if isPositive(x) {
		v := source(x)
		target(v)
	}
}
