package h00

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l01"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m00"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Deep-graph caller: imports both leaf and mid layers, chains
// discharges through several functions.
func Pipeline(x int) int {
	if preds.IsPositive(x) && preds.IsSmall(x) {
		a := l00.Target(x)         // IsPositive discharged
		b := l01.Compound(x)       // IsPositive + IsSmall discharged
		c := m00.CallTarget(a + b) // m00 does its own discharge internally
		return c
	}
	return 0
}
