// proven.Returns on a selector-path subject verifies the predicate
// against whatever facts are in scope at the return site; the
// subject does not have to be a bare identifier.

package main

import "github.com/GiGurra/proven/pkg/proven"

type Wrapper struct {
	Raw int
}

func isPositive(x int) bool { return x > 0 }

func make(w Wrapper) int {
	if isPositive(w.Raw) {
		return proven.Returns(w.Raw, isPositive)
	}
	return 1
}

func main() {
	_ = make(Wrapper{Raw: 5})
}
