// prove.That with a blank value LHS still requires the err-check
// pairing: without `if err != nil { return }`, the err==nil side
// has not been entered and the predicate cannot be trusted. The
// downstream call must fail to discharge.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func run(x int) {
	_, _ = prove.That(x, isPositive) // err discarded — no guard pairing
	accept(x)
}

func main() {
	run(5)
}
