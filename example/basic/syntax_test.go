package basic

import (
	"testing"

	"github.com/GiGurra/proven/pkg/proven"
)

// These tests demonstrate how call sites look under the proven preprocessor.
// Without the preprocessor, Attest and TrustMe are no-op wraps, so every
// scenario below compiles and runs. Under the preprocessor:
//
//   - The scenarios marked "accepted" compile. No runtime checks are emitted
//     for proofs that discharged statically.
//   - The scenarios in the REJECTED block (commented out) would fail the
//     build with a diagnostic pointing at the unproven obligation.
//   - TrustMe scenarios compile and inject a runtime guard that panics on
//     violation. Use at boundaries.

// ---------------------------------------------------------------------------
// (A) Refined[P, T] style — call-site scenarios
// ---------------------------------------------------------------------------

// Accepted: every argument is a literal the checker can discharge trivially.
func TestA_LiteralsProvenStatically(t *testing.T) {
	_ = Transfer(
		proven.Attest[Positive](100),
		proven.Attest[proven.And[NonEmpty, MaxLen280]]("hello"),
		proven.Attest[ValidCurrency]("USD"),
	)
}

// Accepted: the result of FindUserID is already Refined[Positive, int], so
// the proof flows through the call chain without re-attestation.
func TestA_ProofFlowsThroughReturnValue(t *testing.T) {
	id := FindUserID("alice")
	usePositiveInt(id)
}

func usePositiveInt(_ proven.Refined[Positive, int]) {}

// Accepted: flow-sensitive analysis discharges each obligation from the
// preceding conditional.
func TestA_ProofFromPrecedingCheck(t *testing.T) {
	amount := externalInt()
	note := externalString()
	currency := externalString()

	if amount > 0 && len(note) > 0 && len(note) <= 280 && isAllowedCurrency(currency) {
		_ = Transfer(
			proven.Attest[Positive](amount),
			proven.Attest[proven.And[NonEmpty, MaxLen280]](note),
			proven.Attest[ValidCurrency](currency),
		)
	}
}

// Accepted: TrustMe injects a runtime check at the boundary. Appropriate
// when the data came from outside the program and no static proof is
// possible (HTTP body, CLI args, environment variables, DB rows).
func TestA_TrustMeAtBoundary(t *testing.T) {
	amount := externalInt()
	note := externalString()
	currency := externalString()

	_ = Transfer(
		proven.TrustMe[Positive](amount),
		proven.TrustMe[proven.And[NonEmpty, MaxLen280]](note),
		proven.TrustMe[ValidCurrency](currency),
	)
}

// Accepted: a user-defined compound predicate avoids visible combinators.
func TestA_UserDefinedCompoundPredicate(t *testing.T) {
	SetPercent(proven.Attest[SmallPositive](42))
}

// REJECTED under the preprocessor (commented so the file still compiles).
//
//	func TestA_UnprovenRejected(t *testing.T) {
//	    amount := externalInt() // no preceding range check
//	    _ = Transfer(
//	        proven.Attest[Positive](amount), // BUILD ERROR: cannot prove Positive(amount)
//	        proven.Attest[proven.And[NonEmpty, MaxLen280]]("ok"),
//	        proven.Attest[ValidCurrency]("USD"),
//	    )
//	}
//
//	func TestA_PartialProofRejected(t *testing.T) {
//	    note := externalString()
//	    if len(note) > 0 {
//	        // NonEmpty discharges from the if-check; MaxLen280 does not.
//	        _ = Transfer(
//	            proven.Attest[Positive](1),
//	            proven.Attest[proven.And[NonEmpty, MaxLen280]](note), // BUILD ERROR: MaxLen280
//	            proven.Attest[ValidCurrency]("USD"),
//	        )
//	    }
//	}

// ---------------------------------------------------------------------------
// (C) Struct embedding style — call-site scenarios
// ---------------------------------------------------------------------------

// Accepted: every field is a literal the checker discharges trivially.
// The embedded Positive / NonEmpty / MaxLen280 / ValidCurrency fields are
// zero-size proof markers; the checker verifies each obligation for the
// accompanying value field(s).
func TestC_DomainLiteralsProvenStatically(t *testing.T) {
	_ = TransferDomain(
		UserID{V: 1},
		UserID{V: 2},
		Amount{V: 100, Curr: Currency{S: "USD"}},
		Note{S: "hello"},
	)
}

// Accepted: the proof rides inside the struct. Passing a UserID around
// does not erase its proof — unlike Refined, there is no .Unwrap() call
// that strips it.
func TestC_ProofRidesInsideStruct(t *testing.T) {
	alice := lookupUser("alice") // returns UserID with proof baked in
	bob := lookupUser("bob")
	_ = TransferDomain(
		alice, bob,
		Amount{V: 50, Curr: Currency{S: "EUR"}},
		Note{S: "split"},
	)
}

func lookupUser(_ string) UserID {
	// Under the preprocessor, the literal 1 discharges Positive.
	return UserID{V: 1}
}

// Accepted: flow-sensitive analysis discharges obligations in struct
// literals just as it does for Attest calls.
func TestC_DomainProofFromPrecedingCheck(t *testing.T) {
	amount := externalInt()
	note := externalString()
	currency := externalString()

	if amount > 0 && len(note) > 0 && len(note) <= 280 && isAllowedCurrency(currency) {
		_ = TransferDomain(
			UserID{V: 1}, UserID{V: 2},
			Amount{V: amount, Curr: Currency{S: currency}},
			Note{S: note},
		)
	}
}

// REJECTED under the preprocessor (commented so the file still compiles).
//
//	func TestC_ZeroValueRejected(t *testing.T) {
//	    // UserID's zero value is {Positive{}, V: 0}. Positive(0) is false.
//	    // The preprocessor rejects the declaration.
//	    var u UserID // BUILD ERROR: cannot prove Positive(UserID.V) for zero value
//	    _ = u
//	}
//
//	func TestC_InvalidLiteralRejected(t *testing.T) {
//	    // The checker sees V: -5 and rejects the literal.
//	    u := UserID{V: -5} // BUILD ERROR: cannot prove Positive(-5)
//	    _ = u
//	}

// ---------------------------------------------------------------------------
// Helpers (stand-ins for external I/O)
// ---------------------------------------------------------------------------

func externalInt() int       { return 0 }
func externalString() string { return "" }

func isAllowedCurrency(s string) bool {
	switch s {
	case "USD", "EUR", "GBP":
		return true
	}
	return false
}
