// Zero now accepts any comparable type at the runtime API. At
// build time, the literal evaluator recognises the empty string
// "" as the zero value of string and accepts the call without a
// runtime check.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(s string) {
	proven.That(s, proven.Zero)
	_ = s
}

func main() {
	target("") // literal empty string — evaluator sees zero
}
