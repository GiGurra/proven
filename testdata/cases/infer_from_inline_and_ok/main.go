// Inline proven.And inside an infer.From slot flattens to its leaves,
// equivalent to listing them as varargs. Here From(proven.And(isEven,
// isSmallPositive)) is the same rule as From(isEven, isSmallPositive)
// — two premises AND-composed.

package main

import (
	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool      { return x > 0 }
func isEven(x int) bool          { return x%2 == 0 }
func isSmallPositive(x int) bool { return x > 0 && x < 100 }

var _ = infer.From(proven.And(isEven, isSmallPositive)).To(isPositive)

func target(x int) {
	proven.That(x, isPositive)
}

func main() {
	x := 4
	if isEven(x) && isSmallPositive(x) {
		target(x)
	}
}
