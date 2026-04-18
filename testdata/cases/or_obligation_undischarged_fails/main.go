// No fact establishing either disjunct is in scope, so the
// Or-obligation remains undischarged and strict mode emits a
// dedicated "undischarged disjunction" diagnostic.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.Or(isPositive, lessThan100))
}

func main() {
	target(200)
}
