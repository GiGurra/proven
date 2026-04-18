// The runtime boundary validator prove.That establishes a fact
// only on the err == nil branch. This fixture calls the
// obligation-bearing target inside the err != nil branch — where
// the predicate has not been verified — so the preprocessor must
// emit an undischarged-obligation diagnostic.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func handle(raw int) error {
	v, err := prove.That(raw, isPositive)
	if err != nil {
		// prove.That failed; v is the unvalidated input, NOT
		// known to satisfy isPositive. A call to target here
		// should fail the build.
		target(v)
		return err
	}
	return nil
}

func main() {
	_ = handle(-5)
}
