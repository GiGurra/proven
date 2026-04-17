// Package basic demonstrates proven's syntax in two complementary styles:
//
//   (A) Refined[P, T] wrappers for primitives and ad-hoc constraints.
//   (C) Struct embedding for domain types (UserID, TransferAmount, etc.).
//
// Both styles share the same predicate vocabulary below.
package basic

import "github.com/GiGurra/proven/pkg/proven"

// --- Predicate vocabulary ---
//
// Predicates are zero-size struct types. The Check method is optional —
// the static checker doesn't need it; TrustMe does. Predicates are the
// same regardless of which style (A or C) consumes them.

// Positive: x > 0.
type Positive struct{}

func (Positive) Check(x int) bool { return x > 0 }

// NonEmpty: len(s) > 0.
type NonEmpty struct{}

func (NonEmpty) Check(s string) bool { return len(s) > 0 }

// MaxLen280: len(s) <= 280.
type MaxLen280 struct{}

func (MaxLen280) Check(s string) bool { return len(s) <= 280 }

// ValidCurrency: s is an ISO code we support.
type ValidCurrency struct{}

func (ValidCurrency) Check(s string) bool {
	switch s {
	case "USD", "EUR", "GBP":
		return true
	}
	return false
}

// --- (A) Refined[P, T] style ---
//
// Use when the parameter is a bare primitive and no domain struct exists.
// Composition uses proven.And / proven.Or / proven.Not or a user-defined
// compound predicate type with a combined Check.

// Transfer takes a positive amount, a non-empty note of at most 280 bytes,
// and a currency from the supported set.
func Transfer(
	amount proven.Refined[Positive, int],
	note proven.Refined[proven.And[NonEmpty, MaxLen280], string],
	currency proven.Refined[ValidCurrency, string],
) error {
	_ = amount.Unwrap()
	_ = note.Unwrap()
	_ = currency.Unwrap()
	return nil
}

// FindUserID returns a positive user id. The proof flows with the return
// value — callers passing this into another Refined[Positive, int] parameter
// do not need to re-prove anything.
func FindUserID(name string) proven.Refined[Positive, int] {
	_ = name
	// Preprocessor sees 42 and discharges Positive trivially.
	return proven.Attest[Positive](42)
}

// SmallPositive is a user-defined compound predicate: an alternative to
// proven.And[Positive, LessThan100].
type SmallPositive struct{}

func (SmallPositive) Check(x int) bool { return x > 0 && x < 100 }

// SetPercent takes an int in (0, 100) using the compound predicate form.
func SetPercent(p proven.Refined[SmallPositive, int]) { _ = p.Unwrap() }

// --- (C) Struct embedding style ---
//
// Use when the parameter is a domain entity. The struct already exists;
// embedding proofs adds one line per constraint. The proof fields are
// zero-size, so there is no runtime overhead. Access is direct: p.V.

// UserID is a positive int that represents a user. The embedded Positive
// carries the proof. Constructing UserID{V: x} is a proof obligation on x.
type UserID struct {
	Positive
	V int
}

// Note is a string that must be non-empty and under 280 bytes. The two
// embedded predicates compose via struct embedding — no And combinator.
type Note struct {
	NonEmpty
	MaxLen280
	S string
}

// Amount is a positive int in a validated currency. Composition of
// predicates across different *fields* is just multiple named fields,
// not embedding.
type Amount struct {
	Positive      // applies to V
	V    int
	Curr Currency // applies to Curr.S
}

// Currency is a string in the supported ISO set.
type Currency struct {
	ValidCurrency
	S string
}

// TransferDomain is the (C) analog of Transfer. Each struct parameter
// carries its own proofs, and the call site constructs them with literals
// that the preprocessor proves discharge each obligation.
func TransferDomain(from UserID, to UserID, amount Amount, note Note) error {
	_ = from.V
	_ = to.V
	_ = amount.V
	_ = amount.Curr.S
	_ = note.S
	return nil
}
