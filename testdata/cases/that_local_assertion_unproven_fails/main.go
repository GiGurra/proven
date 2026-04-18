// proven.That on a local variable is a compile-time assertion:
// at the call site, the listed predicates must hold on the subject
// given the facts in scope. Here v has no established predicate —
// nothing has proved isPositive on v at this program point — so
// the build fails with a "cannot prove" diagnostic naming the
// predicate, the subject, and the assertion shape.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	v := x + 1
	proven.That(v, isPositive) // v has no isPositive source in scope
	_ = v
}

func main() {
	target(5)
}
