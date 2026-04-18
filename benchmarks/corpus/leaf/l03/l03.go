package l03

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Methods with value receiver.
type Counter struct{ n int }

func (c Counter) Increment(by int) Counter {
	proven.That(by, preds.IsPositive)
	return Counter{n: c.n + by}
}

func (c Counter) Value() int { return c.n }
