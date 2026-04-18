// Nested proven.And flattens fully. proven.And(proven.And(a, b), c)
// decomposes to the three leaves {a, b, c} on ingest, so the target
// discharges when every leaf fact is established by the caller.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func isEven(x int) bool      { return x%2 == 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.And(proven.And(isPositive, isEven), lessThan100))
}

func main() {
	x := 42
	if isPositive(x) && isEven(x) && lessThan100(x) {
		target(x)
	}
}
