package b02

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l05"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/prove"
)

// Boundary for string values — decoded-from-external inputs that
// must satisfy a predicate before they can flow into annotated APIs.
func HandleString(raw string) error {
	s, err := prove.That(raw, preds.IsNonEmpty)
	if err != nil {
		return err
	}
	l05.Accept(s)
	return nil
}

func StartupString(raw string) {
	s := prove.Must(raw, preds.IsNonEmpty, preds.NoWhitespace)
	l05.CompoundString(s)
}
