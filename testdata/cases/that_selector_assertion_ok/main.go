// proven.That works on selector-path subjects the same way it
// works on bare identifiers: a guard on holder.Value establishes
// the fact, and the subsequent proven.That assertion on the same
// path is verified from that source.

package main

import "github.com/GiGurra/proven/pkg/proven"

type PtrHolder struct {
	Value *int
}

func run(holder PtrHolder) {
	if holder.Value != nil {
		proven.That(holder.Value, proven.NonNil) // established by the nil guard
	}
}

func main() {
	run(PtrHolder{})
}
