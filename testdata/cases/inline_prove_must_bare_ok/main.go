// prove.Must called as a bare expression statement (return value
// discarded) is a runtime assertion whose success establishes the
// listed predicates on the first argument's canonical key. The
// following call takes the same argument and discharges without
// re-binding.

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
	x := 5
	prove.Must(x, isPositive)
	accept(x)
}
