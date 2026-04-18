// x -= 1 could push a positive number to zero or negative — the
// analyzer forgets x's facts on any compound assign.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	x := 42
	if proven.Positive(x) {
		x -= 10
		target(x) // cannot prove — might now be non-positive
	}
}
