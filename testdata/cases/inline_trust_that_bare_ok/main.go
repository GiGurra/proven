// trust.That called as a bare expression statement asserts — without
// a runtime check — that the listed predicates hold on the first
// argument. It plants the same facts prove.Must does at the analyzer
// level, so the same inline pattern discharges downstream calls.

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
	x := 5
	trust.That(x, isPositive)
	accept(x)
}
