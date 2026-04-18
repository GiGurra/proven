package h01

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l02"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Many callers of one annotated function in sequence —
// each call discharged independently.
func ManyCalls(a, b, c, d int) int {
	total := 0
	if preds.IsPositive(a) {
		total += l00.Target(a)
	}
	if preds.IsPositive(b) {
		total += l00.Target(b)
	}
	if preds.IsPositive(c) {
		total += l00.Target(c)
	}
	if preds.IsPositive(d) {
		total += l00.Target(d)
	}
	return total
}

// Postcondition chain: l02.MakePositive produces an IsPositive value,
// which flows into l00.Target without an explicit guard.
func NormalizeAndTarget(x int) int {
	p := l02.MakePositive(x)
	return l00.Target(p)
}
