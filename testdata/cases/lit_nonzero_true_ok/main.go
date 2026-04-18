// The bool literal true is not the zero value, so NonZero evaluates
// to true at build time and the call discharges without a runtime
// check.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(b bool) {
	proven.That(b, proven.NonZero)
	_ = b
}

func main() {
	target(true)
}
