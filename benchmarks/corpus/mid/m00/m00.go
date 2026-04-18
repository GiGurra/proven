package m00

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Guard discharge via preceding if-check.
func CallTarget(x int) int {
	if preds.IsPositive(x) {
		return l00.Target(x) // discharged
	}
	return 0
}

// Early-return guard.
func MustBePositive(x int) int {
	if !preds.IsPositive(x) {
		return -1
	}
	return l00.Target(x) // discharged
}

// Literal evaluation is NOT yet supported; use a guard instead.
func Default() int {
	x := 42
	if preds.IsPositive(x) {
		return l00.Target(x)
	}
	return 0
}
