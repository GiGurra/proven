// Package callee declares a precondition-annotated function and
// the predicate that constrains it. The caller package imports
// this and discharges the precondition at its call site via a
// preceding if-check. Under -toolexec=proven the build succeeds.

package callee

import "github.com/GiGurra/proven/pkg/proven"

// IsPositive is exported so the importing package can reference
// the same predicate identity the scanner recorded on Target.
func IsPositive(x int) bool { return x > 0 }

// Target requires IsPositive(amount). Every caller — in this
// package or in any package that imports it — must establish
// that fact before the call site.
func Target(amount int) {
	proven.That(amount, IsPositive)
}
