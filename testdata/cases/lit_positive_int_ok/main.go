// Literal auto-proof: an integer literal passed directly to a
// function whose precondition is proven.Positive is accepted at
// build time — the evaluator parses the literal and sees 42 > 0.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	target(42)
}
