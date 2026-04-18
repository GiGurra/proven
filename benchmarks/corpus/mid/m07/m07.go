package m07

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l07"
)

// Exercises cross-package-local predicate resolution: IsTriple and
// IsFive live in l07, the caller selects them via l07.IsTriple.
func Feed(n int) {
	if l07.IsTriple(n) && l07.IsFive(n) {
		l07.TakeBoth(n)
	}
}

func FeedTriple(n int) {
	if l07.IsTriple(n) {
		l07.TakeTriple(n)
	}
}
