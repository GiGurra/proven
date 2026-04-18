// prove.That with err-check on a local variable establishes the
// predicate on both the LHS ident and the first-arg canonical key.
// A downstream call using either name discharges.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func main() {
	raw := 42
	v, err := prove.That(raw, isPositive)
	if err != nil {
		return
	}
	accept(v)   // fact on v
	accept(raw) // fact on raw (same canonical-key target)
}
