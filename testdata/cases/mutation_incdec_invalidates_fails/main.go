// x-- might make a positive number zero; the analyzer cannot
// reason about numeric bounds, so it conservatively forgets the
// Positive fact on any inc/dec of x.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	x := 42
	if proven.Positive(x) {
		x--
		target(x) // cannot prove — could have become zero
	}
}
