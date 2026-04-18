package callee

import "github.com/GiGurra/proven/pkg/proven"

func IsPositive(x int) bool { return x > 0 }

// Forward has no proven.Returns; the derivation pass stores the
// inferred postcondition in the sidecar so callers in other packages
// pick it up.
func Forward(p int) int {
	proven.That(p, IsPositive)
	return p
}
