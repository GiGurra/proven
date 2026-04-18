// prove.That with a blank value LHS and a non-blank err LHS is a
// common in-place assertion idiom. Once the err-check guard has
// cleared the failure branch, the predicate holds on the first
// argument's canonical key, even though the returned value was
// discarded. Downstream calls on the same argument discharge.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func run(x int) error {
	_, err := prove.That(x, isPositive)
	if err != nil {
		return err
	}
	accept(x)
	return nil
}

func main() {
	_ = run(5)
}
