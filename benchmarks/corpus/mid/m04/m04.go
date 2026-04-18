package m04

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l03"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Method calls with value receiver: c.Increment(x) where x must be positive.
func BumpCounter(c l03.Counter, by int) l03.Counter {
	if preds.IsPositive(by) {
		return c.Increment(by) // discharged
	}
	return c
}

func CountFrom(start, step int) int {
	c := l03.Counter{}
	if preds.IsPositive(step) {
		c = c.Increment(step)
	}
	_ = start
	return c.Value()
}
