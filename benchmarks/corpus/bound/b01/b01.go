package b01

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/trust"
)

// trust.That — no runtime check; programmer takes responsibility.
// Useful when the value is already validated by some external
// mechanism the analyzer cannot see.
func Forward(raw int) int {
	v := trust.That(raw, preds.IsPositive)
	return l00.Target(v)
}

// Multiple predicates injected at once.
func ForwardCompound(raw int) int {
	v := trust.That(raw, preds.IsPositive, preds.IsSmall)
	return l00.Target(v) + l00.TargetSmall(v)
}
