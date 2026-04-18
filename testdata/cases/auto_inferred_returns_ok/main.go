// Auto-inferred postcondition: mid has no explicit proven.Returns,
// but its body establishes isPositive(p) as a precondition and the
// return is the parameter itself — so the analyzer snapshots
// isPositive on the returned identifier and advertises it to callers.
// target requires isPositive and discharges without a guard.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func mid(p int) int {
	proven.That(p, isPositive)
	return p // no proven.Returns; the body's facts on p advertise themselves
}

func target(x int) {
	proven.That(x, isPositive)
}

func main() {
	x := 42
	if isPositive(x) {
		v := mid(x)
		target(v)
	}
}
