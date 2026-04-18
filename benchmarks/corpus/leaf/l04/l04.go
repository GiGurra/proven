package l04

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Pointer receiver — scanner must handle *Bucket.Put.
type Bucket struct{ cap int }

func NewBucket(cap int) *Bucket {
	proven.That(cap, preds.IsPositive)
	return &Bucket{cap: cap}
}

func (b *Bucket) Put(n int) {
	proven.That(n, preds.IsNonNeg)
	b.cap -= n
}
