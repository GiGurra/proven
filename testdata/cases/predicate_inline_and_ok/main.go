// Inline proven.And at an obligation site decomposes into its leaf
// predicates on ingest. The scanner records isPositive and lessThan100
// as two independent obligations on target's parameter; caller's && guard
// establishes each leaf fact, and both obligations discharge.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.And(isPositive, lessThan100))
}

func main() {
	x := 42
	if isPositive(x) && lessThan100(x) {
		target(x)
	}
}
