// Package prove provides runtime boundary validation for Go programs.
//
// proven declares preconditions inside function bodies and has the
// preprocessor discharge them statically. But static proof is
// impossible for data crossing an external boundary (HTTP bodies,
// parsed JSON, CLI arguments, DB rows) — those values' properties are
// not knowable until runtime. prove.That and prove.Must handle that
// case: they run the predicate checks at runtime and return a value
// the preprocessor can treat as proven from that point forward.
//
// Under the proven preprocessor, a successful prove.That call is a
// flow-sensitive fact: the returned value satisfies every predicate
// that was passed. Downstream proven.That calls requiring those same
// predicates on that value discharge automatically, without any
// re-validation.
//
// Use prove.That when the error should flow back to the caller (HTTP
// handlers, decoders). Use prove.Must when a validation failure is
// fatal — startup paths, config loading, tests where bad input is
// programmer error.
package prove

import (
	"github.com/GiGurra/proven/pkg/proven"
)

// That runs runtime predicate checks on v. Returns v with a nil error
// when every predicate holds. On the first failing predicate, returns
// v with a non-nil error that is a proven.Violation naming the failing
// predicate and the offending value. Callers can use errors.As to
// inspect the violation, or simply treat it as any other error.
//
// Successful calls establish a flow-sensitive fact the proven
// preprocessor uses to discharge downstream proven.That obligations on
// the returned value.
func That[T any](v T, preds ...func(T) bool) (T, error) {
	for _, pred := range preds {
		if !pred(v) {
			return v, proven.Violation{Predicate: pred, Value: v}
		}
	}
	return v, nil
}

// Must runs runtime predicate checks on v. Panics with a
// proven.Violation on the first failing predicate. Otherwise returns v
// unchanged.
//
// Use for startup paths where a validation failure is fatal and early
// exit is correct. Use That in request-handling code where failures
// should be returned as errors.
func Must[T any](v T, preds ...func(T) bool) T {
	for _, pred := range preds {
		if !pred(v) {
			panic(proven.Violation{Predicate: pred, Value: v})
		}
	}
	return v
}
