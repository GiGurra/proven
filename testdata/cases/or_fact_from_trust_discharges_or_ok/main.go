// trust.That with an inline proven.Or plants a disjunctive fact
// unconditionally (no runtime check — programmer takes responsibility).
// A downstream Or-obligation with matching alternatives is
// discharged.

package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.Or(isPositive, lessThan100))
}

func main() {
	raw := 200
	v := trust.That(raw, proven.Or(isPositive, lessThan100))
	target(v)
	fmt.Println(v)
}
