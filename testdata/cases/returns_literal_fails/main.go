// proven.Returns(42, isPositive) claims that 42 satisfies
// isPositive without proving it at the Returns site. Before the
// verification fix this silently advertised a postcondition to
// callers who would then treat their result as proven. Strict
// mode refuses: a literal has no flow-state identity and cannot
// carry a fact, so the build fails here. The fix is to wrap with
// trust.That (or prove.Must, or establish the fact via a guard).

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source() int {
	return proven.Returns(42, isPositive)
}

func main() {
	_ = source()
}
