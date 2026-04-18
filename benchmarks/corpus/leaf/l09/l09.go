package l09

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/proven"
)

// Slice-subject obligation.
func FirstOf(xs []int) int {
	proven.That(xs, preds.IsNonEmptyInts)
	return xs[0]
}

func BinarySearch(xs []int, target int) int {
	proven.That(xs, preds.IsSortedInts)
	// ... pretend we do binary search
	return -1
}
