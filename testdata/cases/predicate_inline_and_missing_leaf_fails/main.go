// Inline proven.And decomposes to leaf obligations — and every leaf
// must discharge. Here the caller only establishes isPositive, so
// the lessThan100 leaf is undischarged and the build must fail.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.And(isPositive, lessThan100))
}

func main() {
	x := 42
	if isPositive(x) {
		target(x) // lessThan100 leaf not established
	}
}
