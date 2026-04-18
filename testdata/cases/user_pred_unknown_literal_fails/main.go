// The literal evaluator only recognizes predicates from the proven
// library. A user-defined predicate that happens to be semantically
// identical to proven.Positive still requires a normal proof path —
// guard, prove.Must, trust.That, or an inference rule. Here no
// such path exists, so the build fails.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 } // user predicate, NOT proven.Positive

func target(n int) {
	proven.That(n, isPositive)
}

func main() {
	target(42) // literal eval does not apply — isPositive is not library-known
}
