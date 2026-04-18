package h03

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l03"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l04"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Mixed value-receiver + pointer-receiver callers.
func Orchestrate(capacity, step int) (*l04.Bucket, l03.Counter) {
	var c l03.Counter
	var b *l04.Bucket

	if preds.IsPositive(capacity) {
		b = l04.NewBucket(capacity)
	}
	if preds.IsPositive(step) {
		c = c.Increment(step)
	}
	return b, c
}
