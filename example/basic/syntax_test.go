package basic

import (
	"testing"

	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/proventest"
)

// Scenarios showing how call sites look. Under the preprocessor,
// provable calls compile and the proven.That / proven.Returns checks
// are erased. Without the preprocessor, the calls run as plain runtime
// checks — bad inputs panic.

// 1. Literal arguments — preprocessor discharges every predicate from
// the constant. The runtime check also passes.
func TestLiteralsAccepted(t *testing.T) {
	if err := Transfer(100, "hello", "USD"); err != nil {
		t.Fatal(err)
	}
	SetPercent(50)
}

// 2. Proof flows through a Returns-declared postcondition. A function
// returning FindUserID() carries the fact isPositive(result) into the
// next call site.
func TestProofFlowsThroughReturnValue(t *testing.T) {
	id := FindUserID("alice")
	usePositive(id)
}

func usePositive(x int) {
	proven.That(x, isPositive)
	_ = x
}

// 3. Preceding predicate checks establish facts the preprocessor uses
// to discharge the callee's preconditions. Runtime checks still run
// but are redundant under this flow.
func TestProofFromPrecedingCheck(t *testing.T) {
	amount := externalInt()
	note := externalString()
	currency := externalString()

	if isPositive(amount) && isNonEmpty(note) && maxLen280(note) && validCurrency(currency) {
		_ = Transfer(amount, note, currency)
	}
}

// 4. Early-return guards narrow scope. After the returns, the
// preprocessor treats each guarded predicate as established.
func TestEarlyReturnGuards(t *testing.T) {
	p := externalInt()
	if !isPositive(p) {
		return
	}
	if !lessThan100(p) {
		return
	}
	SetPercent(p)
}

// 5. REJECTED under the preprocessor (commented so this file builds):
//
//	func TestUnprovenRejected(t *testing.T) {
//	    amount := externalInt() // no preceding check
//	    _ = Transfer(amount, "hi", "USD") // preprocessor: cannot prove isPositive(amount)
//	}

// Under the new design, proven.That / proven.Returns never panic at
// runtime in production — their bodies are atCompileTime blocks
// consumed by the preprocessor. Tests can still verify the assertions
// are wired to the intended predicates by opting in via
// proventest.WithChecks, which executes the atCompileTime blocks at
// runtime so a failing predicate panics.

// 6. Verification via WithChecks: ensure Transfer really does refuse a
// negative amount when the check is forced to run.
func TestWiringVerification_TransferRejectsNegativeAmount(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic under WithChecks")
		}
	}()
	proventest.WithChecks(func() {
		_ = Transfer(-5, "hi", "USD")
	})
}

// 7. Verification via WithChecks: ensure Transfer refuses an empty note.
func TestWiringVerification_TransferRejectsEmptyNote(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic under WithChecks")
		}
	}()
	proventest.WithChecks(func() {
		_ = Transfer(10, "", "USD")
	})
}

// 8. Verification via WithChecks: Returns postcondition panics on a
// value that fails the stated predicate.
func TestWiringVerification_ReturnsRejectsBadPostcondition(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic under WithChecks")
		}
	}()
	proventest.WithChecks(func() {
		_ = proven.Returns(-1, isPositive)
	})
}

// 9. Valid inputs do NOT panic even with checks enabled.
func TestWiringVerification_ValidInputsPassUnderWithChecks(t *testing.T) {
	proventest.WithChecks(func() {
		if err := Transfer(100, "hello", "USD"); err != nil {
			t.Fatal(err)
		}
	})
}

// ---------------------------------------------------------------------------
// Stand-ins for external input.
// ---------------------------------------------------------------------------

func externalInt() int       { return 0 }
func externalString() string { return "" }
