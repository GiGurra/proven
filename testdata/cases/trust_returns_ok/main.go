// trust.Returns combines trust.That's no-runtime-check semantics
// with proven.Returns's function-level postcondition advertisement:
// the value is returned unchanged, the predicates are advertised
// as the function's postcondition visible to every caller via the
// sidecar, and — unlike proven.Returns — the site is NOT verified
// at build time. trust.Returns is the escape hatch for literals
// and computed expressions the flow analyzer cannot reason about
// but the programmer knows are sound.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

// The literal 42 trivially satisfies isPositive, but the analyzer
// can't see that — trust.Returns accepts the programmer's word and
// propagates the postcondition to every caller.
func DefaultUserID() int {
	return trust.Returns(42, isPositive)
}

// Consumer: target's precondition discharges via the postcondition
// trust.Returns advertised, so main doesn't need an explicit guard.
func target(userID int) {
	proven.That(userID, isPositive)
}

func main() {
	v := DefaultUserID()
	target(v)
}
