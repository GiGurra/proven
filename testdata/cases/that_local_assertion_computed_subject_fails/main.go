// Computed subjects (arithmetic, call results, and other non-
// trackable shapes) have no canonical key the analyzer can reason
// about across statements, so strict mode still rejects them at
// scan time with the same message as before — only the locked-to-
// parameters restriction was loosened, not the trackable-subject
// requirement.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func main() {
	x := 5
	proven.That(x+1, isPositive) // x+1 is not a trackable subject
	_ = x
}
