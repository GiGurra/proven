// Sad path counterpart to derived_return_chain_depth2_ok: the same
// three-helper chain shape, but the middle helper breaks the chain
// by returning a fresh literal rather than forwarding inner's
// result. middle's derived returns therefore stay empty no matter
// how many fixpoint iterations the discovery loop runs, and outer's
// discharge against target's NonNil obligation must remain
// undischarged. This guards against any future change that
// accidentally lets fixpoint iteration "skip over" a broken link
// (e.g., by reading stale callee state instead of the just-recorded
// derived returns).

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func target(p *int) { proven.That(p, proven.NonNil) }

func main() {
	x := new(int)
	*x = 7
	target(outer(x)) // must fail: middle drops the fact
}

func outer(t *int) *int {
	y := middle(t)
	return y
}

func middle(t *int) *int {
	_ = inner(t)        // inner's NonNil postcondition is established but discarded
	return new(int)     // returns a fresh, non-forwarded value: chain broken here
}

func inner(t *int) *int {
	prove.Must(t, proven.NonNil)
	return t
}
