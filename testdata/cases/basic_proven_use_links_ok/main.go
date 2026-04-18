// A fixture that uses proven.That. Under the preprocessor, the
// pkg/proven compile step is augmented with a synthesized stub that
// provides the _proven_atCompileTime linker symbol as a no-op, so the
// build succeeds and the program links normally. Without the
// preprocessor, this same program fails to link — that is the
// designed behavior of the linker gate, verified by pkg/proven's
// other tests.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func main() {
	proven.That(42, isPositive)
}
