// Calling a proven.NonNil-requiring function with a selector-path
// argument that has no surrounding nil guard must fail the build.
// Verifies that the selector subject is tracked as a distinct key
// and the absence of a source is correctly reported as undischarged.

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
