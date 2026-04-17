package embeddingexperiment

import "testing"

// ---------------------------------------------------------------------------
// Composition: embed multiple proof markers in one struct. This works
// trivially (see embedding_test.go). The harder question is SUBSET
// validation: given a struct with a rich set of proofs, how do we pass
// it to a function that only requires some of them?
//
// Four approaches, with tradeoffs each. All four compile.
// ---------------------------------------------------------------------------

// A rich input struct with three proofs on the same string.
type RichInput struct {
	NonEmpty
	MaxLen280
	ValidCurrency
	S string
}

// ---------------------------------------------------------------------------
// Approach 1 — Explicit narrowing construction at the call site.
// Write a new, narrower struct literal. The preprocessor observes that
// the source string already carries the required proofs and discharges
// the obligation without a runtime check.
// ---------------------------------------------------------------------------

// Narrower type — needs only NonEmpty.
type JustNonEmpty struct {
	NonEmpty
	S string
}

func logJustNonEmpty(_ JustNonEmpty) {}

func TestSubset_ExplicitNarrowing(t *testing.T) {
	rich := RichInput{S: "USD"}
	// One line of ceremony per call. IDE sees plain Go.
	logJustNonEmpty(JustNonEmpty{S: rich.S})
}

// ---------------------------------------------------------------------------
// Approach 2 — Helper method on the rich type.
// Encapsulate the narrowing. Callers write rich.AsJustNonEmpty().
// Preprocessor verifies the implementation is proof-preserving.
// ---------------------------------------------------------------------------

func (r RichInput) AsJustNonEmpty() JustNonEmpty {
	return JustNonEmpty{S: r.S}
}

func TestSubset_HelperMethod(t *testing.T) {
	rich := RichInput{S: "USD"}
	logJustNonEmpty(rich.AsJustNonEmpty())
}

// ---------------------------------------------------------------------------
// Approach 3 — Generic function with interface constraint.
// Proof markers expose an unexported marker method via the interface; the
// function parameter is generic over any T that (a) has the required
// marker methods and (b) exposes an accessor for the underlying value.
// No interface boxing at runtime — T is a concrete type at each call.
// ---------------------------------------------------------------------------

// Each predicate gets a marker method so it can satisfy an interface.
// (We only add markers to the predicates we want to use via Approach 3.)
func (NonEmpty) _isNonEmpty() {}

// The marker interface — satisfied by any type that embeds NonEmpty.
type IsNonEmpty interface{ _isNonEmpty() }

// The accessor interface — satisfied by any string-bearing wrapper that
// defines String() string (or whatever accessor convention we pick).
type StringBearer interface{ Get() string }

// Rich and narrow types both need a Get() method to satisfy StringBearer.
func (r RichInput) Get() string    { return r.S }
func (j JustNonEmpty) Get() string { return j.S }

// Generic Log: accepts any T with NonEmpty proof plus string accessor.
func logGenericNonEmpty[T interface {
	IsNonEmpty
	StringBearer
}](x T) {
	_ = x.Get()
}

func TestSubset_GenericInterfaceConstraint(t *testing.T) {
	rich := RichInput{S: "USD"}
	narrow := JustNonEmpty{S: "hi"}
	logGenericNonEmpty(rich)   // Go infers T = RichInput
	logGenericNonEmpty(narrow) // Go infers T = JustNonEmpty
}

// ---------------------------------------------------------------------------
// Approach 4 — Plain interface parameter.
// Simplest to write; costs a heap allocation plus an indirection per call.
// Useful when the function is not hot-path and you want maximum caller
// flexibility.
// ---------------------------------------------------------------------------

type NEString interface {
	IsNonEmpty
	Get() string
}

func logInterfaceNonEmpty(x NEString) {
	_ = x.Get()
}

func TestSubset_InterfaceValue(t *testing.T) {
	rich := RichInput{S: "USD"}
	narrow := JustNonEmpty{S: "hi"}
	logInterfaceNonEmpty(rich)
	logInterfaceNonEmpty(narrow)
}
