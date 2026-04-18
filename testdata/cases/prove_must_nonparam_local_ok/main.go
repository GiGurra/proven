// prove.Must on a local (non-parameter) variable plants the fact
// on the local's canonical key; a downstream call whose
// precondition matches the same key discharges without the value
// ever having been a function parameter.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func main() {
	v := 42
	prove.Must(v, isPositive)
	accept(v)
}
