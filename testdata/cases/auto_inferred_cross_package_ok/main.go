// Cross-package auto-inference: callee.Forward has no explicit
// proven.Returns but its derived postcondition (IsPositive on the
// returned identifier) lives in the sidecar. Downstream callers in
// this package plant the fact at the assignment site and discharge
// target's obligation.

package main

import (
	"fixture/callee"

	"github.com/GiGurra/proven/pkg/proven"
)

func target(x int) {
	proven.That(x, callee.IsPositive)
}

func main() {
	x := 42
	if callee.IsPositive(x) {
		target(callee.Forward(x)) // nested: Forward's sidecar-advertised postcondition flows into target
	}
}
