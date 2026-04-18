// Selector-path support under a user-defined predicate guard.
// `if isPositive(payload.Amount)` plants an isPositive fact on the
// canonical key "payload.Amount", satisfying the call's precondition
// at the same selector-path subject.

package main

import "github.com/GiGurra/proven/pkg/proven"

type Payload struct {
	Amount int
}

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func main() {
	payload := Payload{Amount: 5}
	if isPositive(payload.Amount) {
		accept(payload.Amount)
	}
}
