// A cross-package fixture: main imports callee and calls its
// precondition-annotated Target under a preceding IsPositive
// guard. The preprocessor reads callee's summary sidecar during
// main's compile and discharges the obligation by matching the
// same-predicate fact that the if-check establishes.

package main

import "fixture/callee"

func main() {
	x := 5
	if callee.IsPositive(x) {
		callee.Target(x)
	}
}
