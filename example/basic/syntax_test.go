package basic

import (
	"testing"

	"github.com/GiGurra/proven/pkg/proven"
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
// runtime — their bodies are atCompileTime blocks consumed by the
// preprocessor. There are therefore no runtime-panic tests; the
// preceding cases only assert that the patterns compile (which with
// the linkstub they also run, vacuously).

// ---------------------------------------------------------------------------
// Stand-ins for external input.
// ---------------------------------------------------------------------------

func externalInt() int       { return 0 }
func externalString() string { return "" }
