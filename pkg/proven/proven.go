package proven

import (
	"fmt"

	_ "unsafe" // for //go:linkname
)

// atCompileTime marks a block that the proven preprocessor discharges
// statically at every call site of the enclosing function. Under the
// preprocessor, each such call site either compiles (with the
// atCompileTime call erased) or fails the build with a diagnostic
// naming the unproven predicate.
//
// Intentionally declared without a Go body: the symbol is supplied by
// the proven preprocessor during the toolexec pass. Without the
// preprocessor, the link step fails with "undefined: _proven_atCompileTime".
//
// Consequence: type-checking is always green (`gopls`, `go vet`, IDEs
// see ordinary Go), but any attempt to actually build a runnable or
// test binary without the preprocessor refuses to link. Nothing inside
// an atCompileTime block ever executes at runtime — the preprocessor
// consumes it, or the link fails.
//
//go:linkname atCompileTime _proven_atCompileTime
func atCompileTime(_ func())

// That declares a parameter precondition. Pass the parameter value and
// one or more predicate functions; every predicate must hold. Multiple
// predicates are AND-composed.
//
// Under the preprocessor, every call site of the enclosing function
// must prove each predicate for the corresponding argument; the That
// call is then erased. Unprovable call sites fail the build.
//
// The block passed to atCompileTime states what must hold: for each
// predicate, pred(v) is true. The block does NOT run at runtime in
// production builds (the default atCompileTime is a no-op); it is
// consumed statically by the preprocessor. Tests may opt in to having
// the block execute at runtime via proventest.WithChecks, turning a
// failing predicate into a panic — useful for verifying that the
// precondition is wired to the intended predicate.
func That[T any](v T, preds ...func(T) bool) {
	atCompileTime(func() {
		for _, pred := range preds {
			if !pred(v) {
				panic(fmt.Sprintf("proven.That: precondition violated on %v", v))
			}
		}
	})
}

// Returns declares a postcondition on the enclosing function's return
// value. Returns the value unchanged. Callers receive the fact that
// every predicate holds on the returned value, available to the
// preprocessor for discharging downstream obligations without re-proof.
//
// Runtime semantics match That: the block does not execute in
// production builds; tests can opt in via proventest.WithChecks.
func Returns[T any](v T, preds ...func(T) bool) T {
	atCompileTime(func() {
		for _, pred := range preds {
			if !pred(v) {
				panic(fmt.Sprintf("proven.Returns: postcondition violated on %v", v))
			}
		}
	})
	return v
}

// All composes predicates into a single predicate that holds when every
// operand holds.
func All[T any](preds ...func(T) bool) func(T) bool {
	return func(v T) bool {
		for _, p := range preds {
			if !p(v) {
				return false
			}
		}
		return true
	}
}

// Any composes predicates into a single predicate that holds when at
// least one operand holds.
func Any[T any](preds ...func(T) bool) func(T) bool {
	return func(v T) bool {
		for _, p := range preds {
			if p(v) {
				return true
			}
		}
		return false
	}
}

// Not inverts a predicate.
func Not[T any](p func(T) bool) func(T) bool {
	return func(v T) bool { return !p(v) }
}
