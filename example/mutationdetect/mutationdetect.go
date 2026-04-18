package mutationdetect

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func NeedsNonNil(intPtr *int) {
	proven.That(intPtr, proven.NonNil)
}

type PtrHolder struct {
	Value *int
}

// Foo takes x as a parameter so the `if x != nil` guard is not
// statically decidable — staticcheck would flag `x := new(int); if
// x != nil` as an always-true comparison, but the point of this
// demo is to show the discharge path under a genuine nil guard.
func Foo(x *int) {
	if x != nil {
		//MakeNil(&x) // makes it not compile
		//x = nil // also makes it not compile
		//Foo2(x) // still compiles with this.
		NeedsNonNil(x)
	}

	holder1 := PtrHolder{}
	if holder1.Value != nil { // works
		NeedsNonNil(holder1.Value)
	}

	holder2 := PtrHolder{}
	prove.Must(holder2.Value, proven.NonNil)
	NeedsNonNil(holder2.Value)

	holder3 := PtrHolder{}
	_, err := prove.That(holder3.Value, proven.NonNil)
	if err != nil {
		return
	}
	NeedsNonNil(holder3.Value)

	holder4 := PtrHolder{}
	NeedsNonNil(ProvesNonNil(holder4.Value))
}

func ProvesNonNil[T any](t *T) *T {
	prove.Must(t, proven.NonNil)
	return t
}

func Foo2[T any](t T) {
	_ = t
}

func MakeNil(intPtrPtr **int) {
	*intPtrPtr = nil
}
