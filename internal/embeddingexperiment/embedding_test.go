package embeddingexperiment

import "testing"

// ---------------------------------------------------------------------------
// Predicates — zero-size struct markers with optional Check methods.
// Defined locally to keep the experiment self-contained.
// ---------------------------------------------------------------------------

type Positive struct{}

func (Positive) Check(x int) bool { return x > 0 }

type NonNegative struct{}

func (NonNegative) Check(x int) bool { return x >= 0 }

type NonEmpty struct{}

func (NonEmpty) Check(s string) bool { return len(s) > 0 }

type MaxLen280 struct{}

func (MaxLen280) Check(s string) bool { return len(s) <= 280 }

type ValidCurrency struct{}

func (ValidCurrency) Check(s string) bool {
	switch s {
	case "USD", "EUR", "GBP":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Primitive wrappers (the kind proven would ship pre-made).
// Each carries a proof marker alongside the value field.
// ---------------------------------------------------------------------------

type PositiveInt struct {
	Positive
	V int
}

type NonEmptyString struct {
	NonEmpty
	S string
}

// NonEmptyBoundedString composes two predicates by embedding both.
type NonEmptyBoundedString struct {
	NonEmpty
	MaxLen280
	S string
}

// ---------------------------------------------------------------------------
// Domain types — proofs attached to a struct that already has meaning.
// ---------------------------------------------------------------------------

type UserID struct {
	Positive
	V int
}

type Currency struct {
	ValidCurrency
	S string
}

// Amount illustrates distinct proof fields bound to distinct value fields.
// Positive applies to V; Currency carries its own ValidCurrency internally.
type Amount struct {
	Positive // applies to V
	V        int
	Curr     Currency
}

// ---------------------------------------------------------------------------
// Functions that require proofs via their parameter types.
// ---------------------------------------------------------------------------

func Transfer(from UserID, to UserID, amount Amount, note NonEmptyBoundedString) error {
	_ = from.V
	_ = to.V
	_ = amount.V
	_ = amount.Curr.S
	_ = note.S
	return nil
}

// FindUserID returns a struct carrying the proof. Callers do not re-prove
// — the struct is UserID, and its embedded Positive marker rides with it.
func FindUserID(_ string) UserID { return UserID{V: 42} }

// ---------------------------------------------------------------------------
// Tests: verify the patterns compile and behave as expected.
// ---------------------------------------------------------------------------

// 1. Struct literals with named fields compile. This is the primary call-site
// shape. gopls sees pure Go; the preprocessor separately verifies literals.
func TestStructLiteralsCompile(t *testing.T) {
	_ = Transfer(
		UserID{V: 1},
		UserID{V: 2},
		Amount{V: 100, Curr: Currency{S: "USD"}},
		NonEmptyBoundedString{S: "hello"},
	)
}

// 2. Proof rides with the struct through call chains. No .Unwrap() tax.
func TestProofRidesInValue(t *testing.T) {
	alice := FindUserID("alice")
	bob := FindUserID("bob")
	_ = Transfer(
		alice, bob,
		Amount{V: 50, Curr: Currency{S: "EUR"}},
		NonEmptyBoundedString{S: "split"},
	)
}

// 3. Direct value access — no method call ceremony.
func TestDirectFieldAccess(t *testing.T) {
	u := UserID{V: 7}
	if u.V != 7 {
		t.Fatalf("want 7, got %d", u.V)
	}
}

// 4. Primitive wrapper usage — the kind of thing proven would ship.
func TestPrimitiveWrapper(t *testing.T) {
	setTimeout(PositiveInt{V: 30})
}

func setTimeout(_ PositiveInt) {}

// 5. Composed predicates — both Check methods exist on the composite via
// embedded access. Calling them explicitly through the embedded name works.
func TestExplicitEmbeddedMethodCalls(t *testing.T) {
	s := NonEmptyBoundedString{S: "hi"}
	if !s.NonEmpty.Check(s.S) {
		t.Fatal("NonEmpty should accept 'hi'")
	}
	if !s.MaxLen280.Check(s.S) {
		t.Fatal("MaxLen280 should accept 'hi'")
	}
}

// 6. Two predicates that BOTH define Check(string) create an ambiguous
// selector on the composite. This is fine — user code won't call Check
// on the composite; it calls each predicate individually (if at all).
//
// The following line would not compile:
//
//	s := NonEmptyBoundedString{S: "hi"}
//	_ = s.Check(s.S) // ambiguous selector: s.NonEmpty.Check vs s.MaxLen280.Check
//
// We rely on the preprocessor to generate runtime checks by naming each
// embedded predicate explicitly, never via the composite.

// 7. Predicates with DIFFERENT Check signatures (int vs string) do not
// collide; Go's method-set rules keep them disjoint.
type IntStringMixed struct {
	Positive // Check(int) bool
	NonEmpty // Check(string) bool
	V        int
	S        string
}

func TestDisjointCheckSignaturesAreNotAmbiguous(t *testing.T) {
	// Still ambiguous at the selector level even though the argument
	// types disambiguate — Go's selector lookup happens before overload
	// resolution, and there is no overload resolution in Go. So this
	// ALSO requires explicit access:
	m := IntStringMixed{V: 5, S: "hi"}
	if !m.Positive.Check(m.V) {
		t.Fatal("Positive should accept 5")
	}
	if !m.NonEmpty.Check(m.S) {
		t.Fatal("NonEmpty should accept 'hi'")
	}
}

// 8. Empty string literal in a non-empty context — this compiles (gopls
// and Go type checker see plain string). Only the preprocessor would
// flag it. Documented here so we remember that the type system alone
// provides NO runtime protection — the preprocessor is load-bearing.
func TestTypeSystemAlonePermitsInvalidLiterals(t *testing.T) {
	// This builds fine without the preprocessor. It would be rejected
	// by the preprocessor because the empty string violates NonEmpty.
	_ = NonEmptyBoundedString{S: ""}
}

// 9. Typo in a proof marker name is a Go compile error — not silent.
// This is the key safety property over doc-comment-based validation:
//
//	type Broken struct {
//	    Postive  // typo — Go: "undefined: Postive"
//	    V int
//	}
//
// Kept commented to keep the package buildable.
