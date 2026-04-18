// A nil guard on holder.Value plants the NonNil fact on that key,
// but a subsequent assignment `holder.Value = nil` emits a Write on
// the root "holder" — which invalidates every fact keyed under the
// "holder" root, including the "holder.Value" fact. The following
// call must therefore fail to discharge proven.NonNil.

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
		holder.Value = nil
		needsNonNil(holder.Value)
	}
}
