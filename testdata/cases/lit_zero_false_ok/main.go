// The bool literal false is the zero value of bool; passing it to
// a Zero-required parameter discharges at build time via the
// literal evaluator without a runtime check.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(b bool) {
	proven.That(b, proven.Zero)
	_ = b
}

func main() {
	target(false)
}
