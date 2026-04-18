// prove.That accepts a runtime predicate, so a function literal
// would "work" at runtime — the check would fire and pass or fail
// correctly. But the analyzer cannot correlate the lambda with any
// downstream proven.That obligation, so the fact it would establish
// is invisible at build time and the downstream call would silently
// stay undischarged. Strict mode rejects the lambda at the prove.That
// site so users are not misled.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	proven.That(x, isPositive)
}

func main() {
	v, err := prove.That(5, func(n int) bool { return n > 0 })
	if err != nil {
		return
	}
	target(v)
}
