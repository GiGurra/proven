package m05

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l04"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Pointer-receiver method calls.
func FillBucket(cap int, amount int) *l04.Bucket {
	if preds.IsPositive(cap) {
		b := l04.NewBucket(cap)
		if preds.IsNonNeg(amount) {
			b.Put(amount)
		}
		return b
	}
	return nil
}
