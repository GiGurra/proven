// After a reassignment forgets x's facts, a fresh guard re-
// establishes them. The build succeeds.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	x := 42
	if proven.Positive(x) {
		x = -1
		if proven.Positive(x) { // re-prove after the mutation
			target(x)
		}
	}
}
