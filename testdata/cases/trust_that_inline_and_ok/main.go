// Inline proven.And inside trust.That decomposes into leaf facts on
// the LHS, same as listing the leaves directly. The programmer
// vouches for each leaf; the scanner never sees a compound fact.

package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, isPositive, lessThan100)
}

func main() {
	raw := 42
	v := trust.That(raw, proven.And(isPositive, lessThan100))
	target(v)
	fmt.Println(v)
}
