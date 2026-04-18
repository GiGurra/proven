// Bare trust.That on a selector-path subject plants the trusted
// predicate on the canonical key directly, letting the downstream
// call discharge without binding to a local first. Covers the
// rewriter's whole-ExprStmt erasure of bare trust.That calls.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

type PtrHolder struct {
	Value *int
}

func needsNonNil(p *int) {
	proven.That(p, proven.NonNil)
}

func main() {
	holder := PtrHolder{}
	trust.That(holder.Value, proven.NonNil)
	needsNonNil(holder.Value)
}
