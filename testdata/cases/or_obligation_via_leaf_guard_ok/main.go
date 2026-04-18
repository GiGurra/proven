// An Or-obligation at proven.That is discharged when ANY single
// alternative is established as a leaf fact. Caller here guards
// only on isPositive and calls target whose obligation is
// proven.Or(isPositive, lessThan100) — the leaf fact isPositive
// alone satisfies the disjunction.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.Or(isPositive, lessThan100))
}

func main() {
	x := 200
	if isPositive(x) {
		target(x)
	}
}
