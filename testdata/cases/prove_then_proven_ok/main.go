// The runtime boundary validator prove.That — on success —
// establishes a flow fact on its returned value. The canonical
// err-check guard precedes the proven.That-annotated callee, and
// the preprocessor discharges the downstream obligation without
// requiring re-validation.

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
		return err
	}
	target(v)
	return nil
}

func main() {
	_ = handle(42)
}
