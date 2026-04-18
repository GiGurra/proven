// Multi-return with guards: isPositive holds on p at every return
// site, so the intersection across returns advertises isPositive to
// callers. No explicit proven.Returns needed.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func clamp(p int) int {
	proven.That(p, isPositive)
	if p > 100 {
		return p // isPositive(p) holds (the pre-seeded fact)
	}
	return p // same fact path
}

func target(x int) { proven.That(x, isPositive) }

func main() {
	x := 42
	if isPositive(x) {
		v := clamp(x)
		target(v)
	}
}
