package l08

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Multiple parameters with different obligations.
func Pair(a int, b int) int {
	proven.That(a, preds.IsPositive)
	proven.That(b, preds.IsNonNeg)
	return a + b
}

func Triple(a, b, c int) int {
	proven.That(a, preds.IsSmall)
	proven.That(b, preds.IsEven)
	proven.That(c, preds.IsOdd)
	return a + b + c
}

func Mixed(s string, n int) int {
	proven.That(s, preds.IsNonEmpty)
	proven.That(n, preds.IsPositive)
	return len(s) + n
}
