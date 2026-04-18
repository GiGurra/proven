// An early-return guard (`if !pred(x) { return }`) establishes
// the negated predicate fact after the guard, discharging the
// downstream proven.That obligation.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func do(x int) {
	if !isPositive(x) {
		return
	}
	target(x)
}

func main() {
	do(5)
}
