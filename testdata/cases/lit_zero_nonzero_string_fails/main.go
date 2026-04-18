// A non-empty string literal is not the zero value of string, so
// Zero evaluates to false and the build fails with a targeted
// diagnostic naming both the predicate and the offending literal.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(s string) {
	proven.That(s, proven.Zero)
	_ = s
}

func main() {
	target("hello")
}
