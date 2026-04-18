// Package proventest provides test-only infrastructure for code that
// uses the proven package. Importing proventest (including as a blank
// import in a *_test.go file) supplies the _proven_atCompileTime symbol
// so the test binary can link without the proven preprocessor.
//
// By default, the provided symbol is a no-op: proven.That and
// proven.Returns blocks are not executed at runtime, matching production
// behavior. Tests that want to verify "my precondition is wired
// correctly — passing bad input would indeed panic" can opt in for a
// scope using proventest.WithChecks.
//
// This package is intended for use in *_test.go files only. Importing
// it from production code defeats the link-time gate proven relies on.
package proventest

import (
	"reflect"
	"sync/atomic"
	"testing"
	_ "unsafe" // for //go:linkname

	"github.com/GiGurra/proven/pkg/proven"
)

// enabled controls whether atCompileTime blocks execute at runtime.
// Defaults to false — matching production, where the preprocessor has
// erased all call sites and nothing runs.
var enabled atomic.Bool

//go:linkname _proven_atCompileTime _proven_atCompileTime
func _proven_atCompileTime(f func()) {
	if enabled.Load() {
		f()
	}
}

// WithChecks runs fn with proven.That / proven.Returns blocks actually
// executing at runtime. A predicate that fails panics with a
// proven.Violation naming the failing predicate and the bad value.
// Use in tests to verify that preconditions and postconditions are
// wired to the intended predicates.
//
// WithChecks flips a global flag, so tests that call it must not run
// concurrently with other code that invokes proven.That / Returns in a
// different goroutine. Within a single non-parallel test, the flag is
// scoped to the fn call.
func WithChecks(fn func()) {
	enabled.Store(true)
	defer enabled.Store(false)
	fn()
}

// AssertFails asserts that running fn under WithChecks causes the given
// predicate to fire — i.e. that the enclosing proven.That or
// proven.Returns call is wired to pred. If fn does not panic, or if a
// different predicate fires, the test fails.
//
// Usage:
//
//	proventest.AssertFails(t, isPositive, func() {
//	    Transfer(-5, "hi", "USD")
//	})
func AssertFails[T any](t *testing.T, pred func(T) bool, fn func()) {
	t.Helper()
	v := capture(t, fn)
	if reflect.ValueOf(pred).Pointer() != reflect.ValueOf(v.Predicate).Pointer() {
		t.Fatalf("expected %s to fire, got %s (value=%v)",
			proven.PredicateName(pred),
			proven.PredicateName(v.Predicate),
			v.Value,
		)
	}
}

// AssertAnyFailure asserts that running fn under WithChecks causes
// some proven predicate to fire. Returns the violation for further
// inspection. Use when the exact predicate does not matter.
func AssertAnyFailure(t *testing.T, fn func()) proven.Violation {
	t.Helper()
	return capture(t, fn)
}

// capture runs fn under WithChecks and returns the proven.Violation
// raised by the first failing predicate. Fails the test if no panic
// occurred, or if the panic was not a proven.Violation (re-raised).
func capture(t *testing.T, fn func()) proven.Violation {
	t.Helper()
	var got proven.Violation
	var raised bool
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if v, ok := r.(proven.Violation); ok {
				got = v
				raised = true
				return
			}
			// Some other panic — re-raise; not ours to eat.
			panic(r)
		}()
		WithChecks(fn)
	}()
	if !raised {
		t.Fatal("expected a proven.Violation, got no panic")
	}
	return got
}
