// Sad path for NonEmptySlice: without a guard, prove.Must, or
// trust.That establishing the predicate, the call fails to
// discharge.

package main

import "github.com/GiGurra/proven/pkg/proven"

func process(xs []int) {
	proven.That(xs, proven.NonEmptySlice)
	_ = xs
}

func main() {
	var xs []int
	process(xs)
}
