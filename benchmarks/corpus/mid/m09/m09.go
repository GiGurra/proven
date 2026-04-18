package m09

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l09"
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
)

// Slice-subject guards.
func Head(xs []int) int {
	if preds.IsNonEmptyInts(xs) {
		return l09.FirstOf(xs)
	}
	return 0
}

func Search(xs []int, t int) int {
	if preds.IsSortedInts(xs) {
		return l09.BinarySearch(xs, t)
	}
	return -1
}
