// Without a bare prove.Must or an equivalent guard, a selector-path
// argument has no NonNil fact established and the call must fail.
// Sad-path counterpart to inline_prove_must_bare_selector_ok.

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
	needsNonNil(holder.Value)
}
