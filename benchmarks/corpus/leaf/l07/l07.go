package l07

import "github.com/GiGurra/proven/pkg/proven"

// Package-local predicates — exercises same-package predicate
// resolution rather than the cross-package import path. Exported
// so callers can guard against them explicitly.
func IsTriple(n int) bool { return n%3 == 0 }
func IsFive(n int) bool   { return n%5 == 0 }

func TakeTriple(n int) {
	proven.That(n, IsTriple)
}

func TakeFive(n int) {
	proven.That(n, IsFive)
}

func TakeBoth(n int) {
	proven.That(n, IsTriple, IsFive)
}
