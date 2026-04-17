package inferenceexperiment

import (
	"testing"

	"github.com/GiGurra/proven/pkg/proven"
)

// Empirical results on Go 1.26.2 (2026-04):
//
//   Case 1 — explicit [P]:                        compiles.
//   Case 2 — infer P from assignment LHS:         FAILS ("cannot infer P").
//   Case 3 — infer P from call-site arg (Refined): FAILS ("cannot infer P").
//   Case 4 — infer B from assignment LHS:         FAILS ("cannot infer B").
//   Case 5 — infer B from call-site arg (plain int): FAILS ("cannot infer B").
//
// Conclusion: Go 1.26 does not propagate expected-return-type context into
// generic type-parameter inference. Even Case 5 — where the argument of
// `foo(x int)` unambiguously fixes B to int — fails. A short
// `proven.In(x)` form is therefore not ergonomically viable without an
// explicit [P]. Users must write proven.In[Positive](x) (or
// proven.Attest[Positive](x) under the current name).
//
// The failing cases are kept below as commented-out code so future Go
// versions can be probed by uncommenting.

// sentinel is the predicate type used by every case.
type sentinel struct{}

// inPT mirrors the signature of proven.Attest:
//
//	In[P any, T any](x T) Refined[P, T]
//
// P is phantom (no argument constrains it); T is pinned by x.
func inPT[P any, T any](x T) proven.Refined[P, T] {
	return proven.Attest[P](x)
}

// inAB is the user's original abstract sketch:
//
//	In[A any, B any](a A) B
//
// B is fully abstract — not even required to be a Refined.
func inAB[A any, B any](a A) B {
	_ = a
	var zero B
	return zero
}

// useRefined and useInt are the callees used for call-site context tests.
func useRefined(_ proven.Refined[sentinel, int]) {}
func useInt(_ int)                               {}

// Case 1 — explicit P, T inferred from argument. Baseline. Compiles.
func TestCase1_ExplicitP(t *testing.T) {
	_ = inPT[sentinel](42)
}

// Case 2 — P inferred from assignment LHS with inPT. FAILS on go1.26.
//
//	func TestCase2_InPT_FromAssignment(t *testing.T) {
//		var r proven.Refined[sentinel, int] = inPT(42) // cannot infer P
//		_ = r
//	}

// Case 3 — P inferred from call-site arg with inPT. FAILS on go1.26.
//
//	func TestCase3_InPT_FromCallContext(t *testing.T) {
//		useRefined(inPT(42)) // cannot infer P
//	}

// Case 4 — fully abstract inAB via assignment. FAILS on go1.26.
//
//	func TestCase4_InAB_FromAssignment(t *testing.T) {
//		var r proven.Refined[sentinel, int] = inAB(42) // cannot infer B
//		_ = r
//	}

// Case 5 — fully abstract inAB via call-site arg, even when the target
// slot is a plain int. FAILS on go1.26 — the strongest negative result,
// since here B is unambiguously int.
//
//	func TestCase5_InAB_FromCallContext(t *testing.T) {
//		useInt(inAB(42)) // cannot infer B
//	}

// Keep the helpers live so this file builds even with Cases 2–5 commented.
var (
	_ = useRefined
	_ = useInt
	_ = inAB[int, int]
)
