// Shadowing a variable in an inner block does not invalidate the
// outer fact. The analyzer's clone-and-restore around block bodies
// means any invalidation inside the inner scope is discarded on
// exit, so the outer x still carries its proven fact.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	x := 42
	if proven.Positive(x) {
		{
			x := -1 // inner x, shadowing — outer fact preserved after
			_ = x
		}
		target(x) // outer x still has Positive
	}
}
