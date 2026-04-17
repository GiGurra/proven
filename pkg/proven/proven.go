// Package proven provides compile-time proven constraints for Go values.
//
// The types and functions in this package are lightweight runtime stubs.
// The real work happens in the proven -toolexec preprocessor, which scans
// call sites, discharges obligations statically, and either rewrites guards
// or rejects the build.
//
// Without the preprocessor, Attest and TrustMe behave identically: they
// wrap a T in Refined[P, T] without checking P. This lets code compile
// and run for development and non-proven paths. Turn the preprocessor on
// by setting GOFLAGS="-toolexec=proven" to get real guarantees.
//
// Two syntactic styles are supported; see docs/parameter-constraint-syntax.md
// for the full discussion:
//
//   (A) Refined[P, T] — a generic wrapper carrying the proof in its type
//       parameter. Use for primitives and ad-hoc constraints.
//
//   (C) Struct embedding — predicate types are zero-size structs; domain
//       types embed them alongside the underlying value(s). Use for
//       domain entities where a struct already exists.
//
// Both styles share the same predicate vocabulary: a predicate is a
// zero-size struct type, optionally implementing Predicate[T] for the
// runtime-guard fallback.
package proven

// Refined is a T for which predicate P has been established.
//
// P is a phantom type parameter; its identity alone carries the proof.
// Two Refined values with different predicate types are distinct types
// even though they share the same runtime representation.
type Refined[P, T any] struct {
	v T
}

// Unwrap returns the underlying value. Once unwrapped, the proof is gone:
// the raw T must be re-attested before flowing back into a refined position.
func (r Refined[P, T]) Unwrap() T { return r.v }

// Attest asserts that x satisfies predicate P. Under the proven
// preprocessor, the assertion must be statically discharged; if the
// checker cannot prove P(x) from the surrounding facts, the build fails.
//
// Without the preprocessor, Attest is a no-op wrap.
func Attest[P, T any](x T) Refined[P, T] {
	return Refined[P, T]{v: x}
}

// TrustMe wraps x in Refined[P, T] with an injected runtime check.
// Use TrustMe at boundaries (deserialization, HTTP handlers, CLI argument
// parsing) where static proof is not possible.
//
// Under the preprocessor, a call to TrustMe is rewritten to:
//
//	if !P.Check(x) { panic("proven: failed P") }
//	Refined[P, T]{v: x}
//
// Without the preprocessor, TrustMe is a no-op wrap — the check does not
// happen. Production builds must run with the preprocessor enabled.
func TrustMe[P, T any](x T) Refined[P, T] {
	return Refined[P, T]{v: x}
}

// Const marks x as a compile-time constant expression. Under the
// preprocessor, x is evaluated during the toolexec pass and substituted
// with its literal value. Without the preprocessor, Const is the identity
// function — the expression is evaluated at runtime.
//
// Const only works on pure expressions: pure functions, literal inputs,
// no I/O. The preprocessor verifies purity and fails the build otherwise.
func Const[T any](x T) T { return x }

// Predicate is an optional interface for predicates that support runtime
// evaluation. The preprocessor's static checker does not require predicates
// to implement this interface; TrustMe does, for the runtime guard path.
type Predicate[T any] interface {
	Check(T) bool
}
