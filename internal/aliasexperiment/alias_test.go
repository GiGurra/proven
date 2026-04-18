package aliasexperiment

import "testing"

// ---------------------------------------------------------------------------
// Empirical findings on Go 1.26.2 (2026-04-18):
//
//   Variant A — `type Where[T any, P any] = T` (generic ALIAS):
//       REJECTED — "cannot use type parameter declared in alias
//       declaration as RHS".
//
//   Variant B — `type Where[T any, P any] T` (generic NAMED TYPE):
//       REJECTED — "cannot use a type parameter as RHS in type
//       declaration".
//
//   Variant C — non-generic per-combination alias (`type PositiveInt = int`):
//       WORKS. Fully transparent to Go's type checker and to gopls.
//       Passes untyped literals, typed variables, and cross-alias
//       assignments without conversion. Costs: no parametricity — one
//       alias per (predicate-set × underlying-type) combination.
//
// Only Variant C is a viable foundation. The Variant A/B sketches are
// kept as comments so future Go versions can be probed.
//
//	type WhereAlias[T any, P any] = T // rejected in go1.26
//	type WhereNamed[T any, P any] T   // rejected in go1.26
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Variant C — the working shape.
//
// Conceptually: each alias name encodes a specific (underlying type,
// predicate set). The preprocessor maintains a registry of alias names
// and their associated constraints. Go sees plain primitives throughout.
// ---------------------------------------------------------------------------

type PositiveInt = int               // predicate registry: x > 0
type SmallPositiveInt = int          // predicate registry: x > 0 && x < 100
type NonEmptyString = string         // predicate registry: len(s) > 0
type NonEmptyBoundedString = string  // predicate registry: len(s) > 0 && len(s) <= 280
type ValidCurrencyString = string    // predicate registry: s ∈ {USD, EUR, GBP}
type Port = uint16                   // predicate registry: 1 <= x

func transfer(amount PositiveInt, note NonEmptyBoundedString, currency ValidCurrencyString) {
	_ = amount
	_ = note
	_ = currency
}

func setPercent(p SmallPositiveInt) { _ = p }

func findUserID() PositiveInt { return 42 }

// ---------------------------------------------------------------------------
// 1. Untyped literals flow through without ceremony.
// ---------------------------------------------------------------------------

func TestVariantC_UntypedLiterals(t *testing.T) {
	transfer(100, "hello", "USD")
	setPercent(50)
}

// ---------------------------------------------------------------------------
// 2. Typed variables flow through without ceremony (aliases are interchangeable).
// ---------------------------------------------------------------------------

func TestVariantC_TypedVariables(t *testing.T) {
	var amount int = 100
	var note string = "hello"
	var currency string = "USD"
	transfer(amount, note, currency)
}

// ---------------------------------------------------------------------------
// 3. Return value of an alias-typed function is just the primitive.
//    Arithmetic on it keeps working (it IS a primitive).
// ---------------------------------------------------------------------------

func TestVariantC_ReturnValueIsPrimitive(t *testing.T) {
	id := findUserID()
	var asInt int = id
	id2 := id + 1
	_ = asInt
	_ = id2
	transfer(id, "ok", "USD")
}

// ---------------------------------------------------------------------------
// 4. Cross-alias assignment works — PositiveInt, SmallPositiveInt, plain
//    int all share the same Go type. The preprocessor would use the
//    alias name on the RHS to track proof provenance and the alias name
//    on the LHS parameter to know what must be proven.
// ---------------------------------------------------------------------------

func TestVariantC_CrossAliasAssignment(t *testing.T) {
	var small SmallPositiveInt = 50
	transfer(small, "ok", "USD") // preprocessor: SmallPositive ⇒ Positive, OK
	setPercent(small)            // exact match
}

// ---------------------------------------------------------------------------
// 5. The cost of flexibility: Go will happily accept a negative int as
//    PositiveInt because to the type checker it is just int. The
//    preprocessor is the SOLE enforcer. Documented here so nobody is
//    surprised.
// ---------------------------------------------------------------------------

func TestVariantC_TypeSystemPermitsViolations(t *testing.T) {
	var bad int = -5
	transfer(bad, "ok", "USD") // would be rejected by preprocessor, not by go vet
}
