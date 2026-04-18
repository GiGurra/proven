// Sad path for selector-subject proven.That: with no guard,
// prove.Must, or trust.That establishing proven.NonNil on
// holder.Value, the assertion cannot be verified and the build
// fails with a targeted diagnostic naming both the predicate and
// the selector path.

package main

import "github.com/GiGurra/proven/pkg/proven"

type PtrHolder struct {
	Value *int
}

func run(holder PtrHolder) {
	proven.That(holder.Value, proven.NonNil) // no source in scope
}

func main() {
	run(PtrHolder{})
}
