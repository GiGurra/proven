package l05

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// String obligations.
func Accept(s string) {
	proven.That(s, preds.IsNonEmpty)
}

func NormalizeLower(s string) string {
	proven.That(s, preds.IsNonEmpty)
	return s
}

func CompoundString(s string) {
	proven.That(s, preds.IsNonEmpty, preds.NoWhitespace)
}
