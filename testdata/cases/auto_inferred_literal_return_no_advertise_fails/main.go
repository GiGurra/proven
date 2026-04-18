// One return carries isPositive on an identifier; the other returns
// a literal. Intersection collapses to nothing (literals carry no
// analyzer facts), so the function advertises no postcondition and
// the caller's target obligation remains undischarged.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func maybe(p int) int {
	proven.That(p, isPositive)
	if p > 100 {
		return 0 // literal — snapshot is empty
	}
	return p // isPositive(p) holds
}

func target(x int) { proven.That(x, isPositive) }

func main() {
	x := 42
	if isPositive(x) {
		target(maybe(x)) // no advertised postcondition on maybe's result
	}
}
