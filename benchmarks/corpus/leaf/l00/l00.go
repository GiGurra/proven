package l00

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

func Target(amount int) int {
	proven.That(amount, preds.IsPositive)
	return amount * 2
}

func TargetSmall(n int) int {
	proven.That(n, preds.IsSmall)
	return n + 1
}

func TargetNonNeg(v int) int {
	proven.That(v, preds.IsNonNeg)
	return v
}
