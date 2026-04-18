// The `nil` identifier is the zero value of every nilable kind
// (pointer, interface, chan, func, map, slice). Passing nil to a
// Zero-required pointer parameter discharges at build time.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(p *int) {
	proven.That(p, proven.Zero)
	_ = p
}

func main() {
	target(nil)
}
