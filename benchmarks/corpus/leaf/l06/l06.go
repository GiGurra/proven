package l06

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

// Postconditions on different return types. proven.Returns requires
// its value argument to be an identifier with the declared
// predicate already a flow-state fact; compound expressions
// (`prefix + "_suffix"`, `n + 1`) cannot carry facts, so we
// introduce a local, trust.That to establish the fact, and return
// the named value.
func BuildName(prefix string) string {
	s := prefix + "_suffix"
	named := trust.That(s, preds.IsNonEmpty)
	return proven.Returns(named, preds.IsNonEmpty)
}

func FromBytes(n int) int {
	proven.That(n, preds.IsInByteRange)
	v := n + 1
	named := trust.That(v, preds.IsPositive)
	return proven.Returns(named, preds.IsPositive)
}
