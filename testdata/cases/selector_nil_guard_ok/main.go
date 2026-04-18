// Selector-path support: a `holder.Value != nil` guard plants the
// NonNil fact on the canonical key "holder.Value". A subsequent call
// taking holder.Value as an argument discharges its proven.NonNil
// obligation without any intermediate binding.

package main

import "github.com/GiGurra/proven/pkg/proven"

type PtrHolder struct {
	Value *int
}

func needsNonNil(p *int) {
	proven.That(p, proven.NonNil)
}

func main() {
	holder := PtrHolder{}
	if holder.Value != nil {
		needsNonNil(holder.Value)
	}
}
