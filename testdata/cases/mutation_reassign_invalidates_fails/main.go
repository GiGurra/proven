// Reassignment invalidates prior facts. After `x = -1` the
// analyzer must forget that x was proven Positive, and the
// downstream target call fails with cannot-prove.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	x := 42
	if proven.Positive(x) {
		x = -1       // reassignment invalidates the Positive fact
		target(x) // cannot prove — x no longer has the fact
	}
}
