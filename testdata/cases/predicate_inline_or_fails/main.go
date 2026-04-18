// Inline proven.Or at an obligation site is v2 scope — the analyzer
// has no disjunctive-fact representation, so a target obligation
// expressed as Or(a, b) could not be discharged soundly. Strict mode
// rejects it with a message pointing the user at inference rules as
// the supported route.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.Or(isPositive, lessThan100))
}

func main() {
	target(5)
}
