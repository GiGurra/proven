package m06

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l05"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// String guards.
func Ingest(s string) {
	if preds.IsNonEmpty(s) {
		l05.Accept(s)
	}
}

// Two-predicate guard for a two-predicate obligation.
func Strict(s string) {
	if preds.IsNonEmpty(s) && preds.NoWhitespace(s) {
		l05.CompoundString(s)
	}
}
