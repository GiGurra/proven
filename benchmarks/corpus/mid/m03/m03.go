package m03

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	// rules is imported so the inference rules are in scope via
	// the sidecar; the _ alias advertises the intent.
	_ "github.com/GiGurra/proven/benchmarks/corpus/rules"
)

// Discharge via declared inference rules.
// rules declares: IsSmallPositive ⇒ IsPositive.
// A guard of IsSmallPositive discharges the IsPositive obligation.
func ViaInference(x int) int {
	if preds.IsSmallPositive(x) {
		return l00.Target(x) // IsPositive discharged via inference
	}
	return 0
}

// Chain: IsSmallPositive ⇒ IsSmall (one hop) ⇒ ... (no, just one hop;
// rules doesn't chain through beyond this). Still a good exercise of
// the backward-chaining path.
func ViaInferenceSmall(x int) int {
	if preds.IsSmallPositive(x) {
		return l00.TargetSmall(x)
	}
	return 0
}
