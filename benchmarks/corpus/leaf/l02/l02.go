package l02

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Postcondition via proven.Returns.
func Clamp(x int) int {
	if x < 0 {
		return proven.Returns(0, preds.IsNonNeg)
	}
	if x > 100 {
		return proven.Returns(100, preds.IsSmall)
	}
	return proven.Returns(x, preds.IsNonNeg)
}

func MakePositive(x int) int {
	if x > 0 {
		return proven.Returns(x, preds.IsPositive)
	}
	return proven.Returns(1, preds.IsPositive)
}
