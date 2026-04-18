// proven.Not at an obligation site still fails the build — a
// negation-fact representation is v2 scope.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	proven.That(x, proven.Not(isPositive))
}

func main() {
	target(5)
}
