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
	"sync/atomic"
	_ "unsafe" // for //go:linkname
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
// executing at runtime. A predicate that fails panics (see the body of
// proven.That for the panic value format). Use in tests to verify that
// preconditions and postconditions are wired to the intended predicates.
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
