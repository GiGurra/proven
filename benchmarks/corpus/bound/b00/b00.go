package b00

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/prove"
)

// prove.That as an external-boundary validator — the preprocessor
// plants the fact on the err == nil side of the error check.
func Accept(raw int) (int, error) {
	v, err := prove.That(raw, preds.IsPositive)
	if err != nil {
		return 0, err
	}
	// After the err check v is known-IsPositive.
	return l00.Target(v), nil
}

// prove.Must — unconditional fact at the call site.
func MustAccept(raw int) int {
	v := prove.Must(raw, preds.IsPositive)
	return l00.Target(v)
}
