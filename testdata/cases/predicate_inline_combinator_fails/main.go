// proven.And composes named predicates into a new func(T) bool at
// runtime, but the composite value has no package+name identity
// (it is an anonymous function result), so the scanner cannot use
// it for cross-package discharge. In strict mode this fails the
// build instead of silently dropping the obligation — the user can
// assign the combinator to a package-level var and reference that
// name instead.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }

func target(x int) {
	proven.That(x, proven.And(isPositive, lessThan100))
}

func main() {
	target(5)
}
