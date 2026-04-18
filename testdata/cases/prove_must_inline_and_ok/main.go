// Inline proven.And at a fact-declaration site (prove.Must) plants
// each leaf predicate as a separate fact on the LHS, so downstream
// obligations that require any of the leaves discharge without extra
// work. prove.Must(raw, proven.And(a, b)) reads the same as
// prove.Must(raw, a, b).

package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, isPositive, lessThan100)
}

func main() {
	raw := 42
	v := prove.Must(raw, proven.And(isPositive, lessThan100))
	target(v)
	fmt.Println(v)
}
