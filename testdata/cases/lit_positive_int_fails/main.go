// -5 fails proven.Positive at build time — the evaluator parses
// the unary-minus literal and reports EvalFails, so the normal
// cannot-prove diagnostic fires.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func main() {
	target(-5)
}
