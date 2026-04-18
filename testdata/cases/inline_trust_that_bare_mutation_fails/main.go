// A bare trust.That plants the trusted fact on its first-arg key.
// A subsequent mutation to the key's root invalidates the planted
// fact; the downstream call must fail to discharge.

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
	holder.Value = nil // writes to holder.Value; invalidates holder-rooted facts
	needsNonNil(holder.Value)
}
