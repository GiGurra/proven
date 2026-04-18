package mutationdetect

import (
	"github.com/GiGurra/proven/pkg/proven"
)

func NeedsNonNil(intPtr *int) {
	proven.That(intPtr, proven.NonNil)
}

func Foo() {
	x := new(1)
	if x != nil {
		//MakeNil(&x) // makes it not compile
		//x = nil // also makes it not compile
		//Foo2(x) // still compiles with this.
		NeedsNonNil(x)
	}
}

func Foo2[T any](t T) {
	_ = t
}

func MakeNil(intPtrPtr **int) {
	*intPtrPtr = nil
}
