// proven.That on a non-parameter subject: the local variable v is
// an identifier but not a parameter of target, so the analyzer
// cannot advertise the obligation as something callers of target
// must discharge. Strict mode refuses to silently drop the
// obligation.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	v := x + 1
	proven.That(v, isPositive) // v is not a parameter
	_ = v
}

func main() {
	target(5)
}
