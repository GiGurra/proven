// prove.That with an inline proven.Or plants a disjunctive fact on
// the checked value. A downstream obligation shaped as the same Or
// is discharged by structural match — neither disjunct holds
// individually as a leaf fact, but the whole disjunction does.

package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.Or(isPositive, lessThan100))
}

func main() {
	raw := 200
	v, err := prove.That(raw, proven.Or(isPositive, lessThan100))
	if err != nil {
		return
	}
	target(v)
	fmt.Println(v)
}
