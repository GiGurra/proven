// Explicit proven.Returns now flows through nested calls too. The
// inner call's advertised postcondition discharges the outer call's
// obligation without the user having to bind an intermediate.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func guard(p int) int {
	proven.That(p, isPositive)
	return proven.Returns(p, isPositive)
}

func target(x int) { proven.That(x, isPositive) }

func main() {
	x := 42
	if isPositive(x) {
		target(guard(x))
	}
}
