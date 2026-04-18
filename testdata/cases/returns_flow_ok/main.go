// A proven.Returns-annotated value flows forward as a discharged
// fact on the assignment's LHS. The downstream target receives an
// already-proven argument.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source() int {
	return proven.Returns(42, isPositive)
}

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	v := source()
	target(v)
}
