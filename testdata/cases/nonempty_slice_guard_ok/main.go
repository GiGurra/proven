// NonEmptySlice used as a guard predicate: an `if
// proven.NonEmptySlice(xs)` narrows the then-branch so a downstream
// callee requiring the same predicate discharges against the
// guard-planted fact.

package main

import "github.com/GiGurra/proven/pkg/proven"

func process(xs []int) {
	proven.That(xs, proven.NonEmptySlice)
	_ = xs[0]
}

func run(xs []int) {
	if proven.NonEmptySlice(xs) {
		process(xs)
	}
}

func main() {
	run([]int{1, 2, 3})
}
