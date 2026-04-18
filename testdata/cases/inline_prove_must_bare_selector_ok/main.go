// The bare-inline prove.Must idiom extends to selector-path subjects
// under the canonical-expression-key widening: asserting
// proven.NonNil on holder.Value in-place lets the following call on
// holder.Value discharge directly, without binding to a local first.

package main

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

type PtrHolder struct {
	Value *int
}

func needsNonNil(p *int) {
	proven.That(p, proven.NonNil)
}

func main() {
	holder := PtrHolder{}
	prove.Must(holder.Value, proven.NonNil)
	needsNonNil(holder.Value)
}
