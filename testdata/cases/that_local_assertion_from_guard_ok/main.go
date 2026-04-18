// proven.That on a local variable whose predicate has been
// established by a preceding guard passes silently: the assertion
// is a consistency check the analyzer resolves from facts already
// in scope. No caller obligation is created — v is not a parameter.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	v := x
	if isPositive(v) {
		proven.That(v, isPositive) // isPositive is established on v by the guard
		_ = v
	}
}

func main() {
	target(5)
}
