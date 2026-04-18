// proven.Returns(x, isPositive) on a parameter x whose precondition
// does NOT include isPositive. The flow state has no fact for
// isPositive(x), so the postcondition would be advertised to
// callers without ever being proved. The build must fail here.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source(x int) int {
	// No guard, no precondition, no prove/trust — x could be -5.
	return proven.Returns(x, isPositive)
}

func main() {
	_ = source(42)
}
