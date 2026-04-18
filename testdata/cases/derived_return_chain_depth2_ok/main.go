// Regression: a chain of three helpers that forward each other's
// auto-derived postconditions, declared top-down (caller above
// callee). Tests that the discovery loop iterates beyond a single
// pass — two passes (the previous fix) would only propagate the
// innermost helper's [P] up one hop, leaving outer's derived empty
// and main's discharge unable to find P.
//
// File order is intentional: outer (calls middle), middle (calls
// inner), inner (establishes the fact). With a single discovery
// pass and this ordering, outer is analyzed when middle still has
// derived = []. The fixpoint loop must run again so middle picks up
// inner's [P], and a third time so outer picks up middle's [P].

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func target(p *int) { proven.That(p, proven.NonNil) }

func main() {
	x := new(int)
	*x = 7
	target(outer(x))
}

func outer[T any](t *T) *T {
	y := middle(t)
	return y
}

func middle[T any](t *T) *T {
	y := inner(t)
	return y
}

func inner[T any](t *T) *T {
	prove.Must(t, proven.NonNil)
	return t
}
