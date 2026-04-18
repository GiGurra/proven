package l01

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Multi-predicate: AND-composed.
func Compound(x int) int {
	proven.That(x, preds.IsPositive, preds.IsSmall)
	return x
}

func Byte(b int) int {
	proven.That(b, preds.IsInByteRange)
	return b
}

func Odd(n int) int {
	proven.That(n, preds.IsOdd)
	return n + 1
}
