// Regression: same-package caller declared ABOVE its callee where the
// callee's postcondition is analyzer-derived (via prove.Must) rather
// than explicit proven.Returns.
//
// Prior to the two-pass analyzer, this failed because the single-pass
// loop over FuncDecls analyzed main (and its nested-call discharge
// against helper) before helper's DerivedReturnPreds had been
// recorded. An explicit proven.Returns worked only because the scanner
// pre-pass populates ReturnPreds before analysis starts; the
// derived-return path had no such pre-pass.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func target(p *int) { proven.That(p, proven.NonNil) }

func main() {
	x := new(42)
	target(helper(x)) // helper declared below — must still discharge
}

// helper is intentionally below main: its DerivedReturnPreds (NonNil
// on t, from prove.Must) must be recorded before main's discharge
// check runs, which requires the analyzer to take at least two
// passes over the package.
func helper[T any](t *T) *T {
	prove.Must(t, proven.NonNil)
	return t
}
