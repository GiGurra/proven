package h04

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l05"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l08"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Mixed-type obligations (string + int) in one call.
func Process(s string, n int) int {
	if preds.IsNonEmpty(s) && preds.IsPositive(n) {
		return l08.Mixed(s, n)
	}
	return 0
}

// Fan-out: one input, several annotated callees.
func FanOut(s string) {
	if preds.IsNonEmpty(s) {
		l05.Accept(s)
		_ = l05.NormalizeLower(s)
	}
}
