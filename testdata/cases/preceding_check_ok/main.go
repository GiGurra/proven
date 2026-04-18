// A preceding if-check establishes the predicate as a fact in
// the caller's flow state, discharging the callee's proven.That
// obligation at the nested call site.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	x := 5
	if isPositive(x) {
		target(x)
	}
}
