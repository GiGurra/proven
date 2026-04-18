// trust.That on a selector-path argument plants the trusted fact on
// the assignment's LHS identifier. The LHS is bound from a selector
// path, but the FACT lands on the bare ident — the downstream call
// uses that ident directly.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

type Payload struct {
	Raw *int
}

func needsNonNil(p *int) {
	proven.That(p, proven.NonNil)
}

func main() {
	payload := Payload{}
	v := trust.That(payload.Raw, proven.NonNil)
	needsNonNil(v)
}
