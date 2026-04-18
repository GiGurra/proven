package l02

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

// Postcondition via proven.Returns. Each Returns-bearing value must
// have the declared predicate established in the flow state before
// it is passed to Returns. Literals don't have flow-state identity,
// so we either guard the value first or use trust.That to assert
// the fact on our word (safe for compile-time constants we can see
// satisfy the predicate).
func Clamp(x int) int {
	if x < 0 {
		zero := trust.That(0, preds.IsNonNeg)
		return proven.Returns(zero, preds.IsNonNeg)
	}
	if x > 100 {
		hundred := trust.That(100, preds.IsSmall)
		return proven.Returns(hundred, preds.IsSmall)
	}
	// x in the fallthrough branch has both negations of the
	// preceding guards: x >= 0 (from !(x < 0)) and x <= 100
	// (from !(x > 100)). Neither is expressible as a pred call
	// the analyzer recognizes, so trust.That is the honest path.
	v := trust.That(x, preds.IsNonNeg)
	return proven.Returns(v, preds.IsNonNeg)
}

func MakePositive(x int) int {
	if preds.IsPositive(x) {
		return proven.Returns(x, preds.IsPositive)
	}
	one := trust.That(1, preds.IsPositive)
	return proven.Returns(one, preds.IsPositive)
}
