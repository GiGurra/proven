// trust.That asserts a predicate on a local without a runtime
// check; a following proven.That on the same local then verifies
// the analyzer really has that fact in scope. The two together
// form a declarative "I claim this, and I double-check the
// compiler agrees I claim it" idiom useful for documenting
// invariants inside function bodies.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

func main() {
	v := 42
	trust.That(v, isPositive)
	proven.That(v, isPositive) // satisfied by the trust.That source above
	_ = v
}
