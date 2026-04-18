// main calls callee.Target without establishing the precondition
// callee.IsPositive. The preprocessor must read callee's sidecar,
// see the unmet obligation at main's call site, and fail the
// build with a cross-package diagnostic.

package main

import "fixture/callee"

func main() {
	x := 5
	callee.Target(x)
}
