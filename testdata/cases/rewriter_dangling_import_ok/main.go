// The rewriter erases proven.That, but the predicate `isPositive`
// lives in an imported helper package whose ONLY reference in this
// file is inside that erased call. Before the fix, the resulting
// rewritten source read `"helper" imported and not used` and the
// compile failed. The rewriter now emits a sentinel so each
// helper-package import that appears inside erased spans stays in
// use.

package main

import (
	"fmt"

	"fixture/helper"

	"github.com/GiGurra/proven/pkg/proven"
)

func target(n int) int {
	proven.That(n, helper.IsPositive)
	return n * 2
}

func main() {
	x := 5
	if helper.IsPositive(x) {
		fmt.Println(target(x))
	}
}
