// trust.That asserts — without any runtime verification — that
// the value satisfies the predicate. The analyzer treats the
// assertion as a fact on the LHS, so the downstream obligation-
// bearing call discharges and the build succeeds.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	raw := 5
	// trust.That is a contract between the programmer and the
	// analyzer: no runtime check is emitted, but every
	// downstream call sees v as satisfying isPositive.
	v := trust.That(raw, isPositive)
	target(v)
}
