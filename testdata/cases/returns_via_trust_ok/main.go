// trust.That establishes a fact on the LHS of the assignment, so a
// downstream proven.Returns can reference the value by name and the
// analyzer sees the predicate as already-established. This is the
// natural shape for "I know this constant (or locally-derived
// value) satisfies the predicate; take my word for it and advertise
// the postcondition to every caller of this function."

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

func source() int {
	v := trust.That(42, isPositive)
	return proven.Returns(v, isPositive)
}

func target(v int) {
	proven.That(v, isPositive)
}

func main() {
	v := source()
	target(v)
}
