// The blank-value-LHS idiom extends to selector-path arguments: the
// predicate rides the canonical key "holder.Value" through the
// err-check guard without any intermediate binding.

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

func run(holder PtrHolder) error {
	_, err := prove.That(holder.Value, proven.NonNil)
	if err != nil {
		return err
	}
	needsNonNil(holder.Value)
	return nil
}

func main() {
	_ = run(PtrHolder{})
}
