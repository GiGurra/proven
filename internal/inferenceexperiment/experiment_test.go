package inferenceexperiment

import "testing"

// Empirical results on Go 1.26.2 (2026-04):
//
//   Case 1 — explicit [P]:                        compiles.
//   Case 2 — infer P from assignment LHS:         FAILS ("cannot infer P").
//   Case 3 — infer P from call-site arg (Refined): FAILS ("cannot infer P").
//   Case 4 — infer B from assignment LHS:         FAILS ("cannot infer B").
//   Case 5 — infer B from call-site arg (plain int): FAILS ("cannot infer B").
//
// Conclusion: Go 1.26 does not propagate expected-return-type context
// into generic type-parameter inference. This is the reason proven's
// design does not rely on a phantom-typed wrapper function like
// In[P](x) or Refined[P, T]: users would have to spell out [P] at
// every call. The public API instead uses runtime-checkable assertions
// inside the function body (proven.That / proven.Returns).
//
// The failing cases are kept below as commented code so that future
// Go versions can be probed by uncommenting.

// Local Refined type and Attest helper — mirrors the abandoned API
// shape purely to exercise Go's inference. No import of pkg/proven.
type Refined[P, T any] struct{ v T }

func attest[P, T any](x T) Refined[P, T] { return Refined[P, T]{v: x} }

// sentinel is the predicate type used by every case.
type sentinel struct{}

// inPT mirrors the abandoned Attest signature: phantom P, pinned T.
func inPT[P any, T any](x T) Refined[P, T] {
	return attest[P](x)
}

// inAB is the original abstract sketch: B fully unconstrained.
func inAB[A any, B any](a A) B {
	_ = a
	var zero B
	return zero
}

func useRefined(_ Refined[sentinel, int]) {}
func useInt(_ int)                        {}

// Case 1 — explicit P, T inferred from argument. Compiles.
func TestCase1_ExplicitP(t *testing.T) {
	_ = inPT[sentinel](42)
}

// Case 2 — P inferred from assignment LHS with inPT. FAILS on go1.26.
//
//	func TestCase2_InPT_FromAssignment(t *testing.T) {
//		var r Refined[sentinel, int] = inPT(42) // cannot infer P
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
//		var r Refined[sentinel, int] = inAB(42) // cannot infer B
//		_ = r
//	}

// Case 5 — fully abstract inAB via call-site arg, even when the target
// slot is a plain int. FAILS on go1.26.
//
//	func TestCase5_InAB_FromCallContext(t *testing.T) {
//		useInt(inAB(42)) // cannot infer B
//	}

// Keep helpers alive so this file builds with Cases 2–5 commented.
var (
	_ = useRefined
	_ = useInt
	_ = inAB[int, int]
)
