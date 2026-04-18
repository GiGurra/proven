package m08

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l08"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Two-parameter obligations in one call.
func CallPair(a, b int) int {
	if preds.IsPositive(a) && preds.IsNonNeg(b) {
		return l08.Pair(a, b)
	}
	return 0
}

// Three-parameter: all three facts in one &&-conjoined guard.
func CallTriple(a, b, c int) int {
	if preds.IsSmall(a) && preds.IsEven(b) && preds.IsOdd(c) {
		return l08.Triple(a, b, c)
	}
	return 0
}

func CallMixed(s string, n int) int {
	if preds.IsNonEmpty(s) && preds.IsPositive(n) {
		return l08.Mixed(s, n)
	}
	return 0
}
