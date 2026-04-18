package l06

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Postconditions on different return types.
func BuildName(prefix string) string {
	return proven.Returns(prefix+"_suffix", preds.IsNonEmpty)
}

func FromBytes(n int) int {
	proven.That(n, preds.IsInByteRange)
	return proven.Returns(n+1, preds.IsPositive)
}
