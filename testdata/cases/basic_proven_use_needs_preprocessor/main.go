// A fixture that uses proven.That. With the current no-op preprocessor
// stub, the build is expected to FAIL at link time because the
// _proven_atCompileTime symbol is not supplied. Once the preprocessor
// learns to inject the symbol into pkg/proven's compilation, this
// fixture will need to be replaced with a more specific test.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func main() {
	proven.That(42, isPositive)
}
