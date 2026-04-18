// The inner call advertises no postcondition — neither explicit
// proven.Returns nor a derivable one (the body establishes nothing on
// the returned identifier). The outer target's obligation therefore
// remains undischarged at the nested call site.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func blackbox(p int) int {
	return p + 1 // no facts seeded, computed return — empty advertised postcondition
}

func target(x int) { proven.That(x, isPositive) }

func main() {
	x := 42
	if isPositive(x) {
		target(blackbox(x)) // blackbox advertises nothing — target's isPositive undischarged
	}
}
