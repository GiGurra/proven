// A function literal passed to trust.That has no package+name
// identity so the analyzer cannot correlate the lambda with any
// downstream proven.That that requires the same predicate. Strict
// mode rejects it instead of silently leaving the would-be fact
// invisible.

package main

import "github.com/GiGurra/proven/pkg/trust"

func main() {
	_ = trust.That(5, func(n int) bool { return n > 0 })
}
