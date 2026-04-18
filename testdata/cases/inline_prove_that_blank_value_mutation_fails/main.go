// A blank-value prove.That plants the fact on the first-arg key,
// but a subsequent mutation to that key's root invalidates the
// planted fact. The downstream call must fail to discharge.

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
	holder.Value = nil // writes to holder.Value; invalidates holder-rooted facts
	needsNonNil(holder.Value)
	return nil
}

func main() {
	_ = run(PtrHolder{})
}
