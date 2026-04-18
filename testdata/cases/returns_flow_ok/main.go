// A proven.Returns-annotated value flows forward as a discharged
// fact on the assignment's LHS. The downstream target receives an
// already-proven argument.
//
// Note that proven.Returns verifies its own value argument against
// the flow-state facts, so source must itself establish isPositive
// on the value before returning it. Here the function's declared
// precondition does the work: `proven.That(x, isPositive)` seeds
// the analyzer's fact set with isPositive(x) at the start of
// source's body, which makes `return proven.Returns(x, isPositive)`
// valid. The caller (main) discharges source's precondition with
// a guard.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source(x int) int {
	proven.That(x, isPositive)
	return proven.Returns(x, isPositive)
}

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	x := 42
	if isPositive(x) {
		v := source(x)
		target(v)
	}
}
