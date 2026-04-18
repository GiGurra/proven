// rules is a package that declares inference rules but has no
// proven.That / proven.Returns obligations of its own. Before the
// Phase 9 fix, the preprocessor's planUserPackage short-circuit
// skipped writing a sidecar for packages with no local Funcs —
// losing the rule declarations for every downstream package. The
// fix checks len(sum.Rules) in the same short-circuit.
package rules

import (
	"fixture/preds"

	"github.com/GiGurra/proven/pkg/infer"
)

var _ = infer.From(preds.IsSmallPositive).To(preds.IsPositive)
