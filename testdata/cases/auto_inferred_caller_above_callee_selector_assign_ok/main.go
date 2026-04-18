// Regression: same pattern as auto_inferred_caller_above_callee_ok,
// but the helper's result is assigned back to a selector path before
// being passed on. Exercises the assignment-plant path (not just the
// nested-call virtual-plant) under the two-pass analyzer.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

type holder struct {
	Value *int
}

func target(p *int) { proven.That(p, proven.NonNil) }

func main() {
	h := holder{Value: new(7)}
	h.Value = helper(h.Value)
	target(h.Value)
}

func helper[T any](t *T) *T {
	prove.Must(t, proven.NonNil)
	return t
}
