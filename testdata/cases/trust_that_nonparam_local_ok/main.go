// trust.That on a local variable injects the predicate as a fact
// without a runtime check. A downstream call on the local
// discharges. No caller / no parameter involved.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func main() {
	v := trust.That(42, isPositive)
	accept(v)
}
