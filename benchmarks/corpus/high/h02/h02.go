package h02

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m03"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	// Rules in scope — IsSmallPositive ⇒ IsPositive.
	_ "github.com/GiGurra/proven/benchmarks/corpus/rules"
)

// Inference across two packages: the guard uses IsSmallPositive,
// the target requires IsPositive, and the rules package provides
// the implication via its sidecar.
func DeepInference(x int) int {
	if preds.IsSmallPositive(x) {
		return l00.Target(x) + m03.ViaInference(x)
	}
	return 0
}
