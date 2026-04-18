// trust.Returns on a selector-path subject advertises the
// postcondition on the wrapped value without a runtime check, like
// proven.Returns but without compile-time verification. Works on
// any trackable subject, not just bare identifiers / parameters.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

type Wrapper struct {
	Raw int
}

func isPositive(x int) bool { return x > 0 }

func take(x int) {
	proven.That(x, isPositive)
}

func defaultVal(w Wrapper) int {
	return trust.Returns(w.Raw, isPositive)
}

func main() {
	v := defaultVal(Wrapper{Raw: 5})
	take(v)
}
