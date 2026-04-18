package m01

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l01"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Conjoined guard (&&) — both facts in the body.
func CallBoth(x int) int {
	if preds.IsPositive(x) && preds.IsSmall(x) {
		_ = l00.Target(x)    // IsPositive discharges
		return l01.Compound(x) // IsPositive + IsSmall both discharge
	}
	return 0
}

func NegateGuard(x int) int {
	if preds.IsNegative(x) {
		return 0
	}
	// After a negate-then-return guard the negated fact holds —
	// but IsNegative→¬IsPositive isn't the shape the analyzer
	// recognizes. So we still need an explicit positive guard.
	if preds.IsPositive(x) {
		return l00.Target(x)
	}
	return 0
}
