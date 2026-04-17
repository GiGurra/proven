# Design: interface-based proof signatures + struct-embedded proof markers

This is the current design for proven. It supersedes the A+C decision recorded in [`parameter-constraint-syntax.md`](parameter-constraint-syntax.md); that file remains for historical analysis of alternatives considered.

## The core pattern

Four pieces fit together:

1. **Predicate types** are zero-size struct markers carrying two method annotations:
   - `Check(T) bool` — the runtime predicate body, used by `TrustMe` guards and the preprocessor when it needs a value for diagnostics.
   - An unexported marker method `_isX()` declaring membership in a corresponding marker interface.

2. **Marker interfaces** are one-method interfaces matching each predicate's marker method. Any type that embeds a predicate struct inherits the marker method by Go's method-promotion rules and thus satisfies the interface. `gopls` understands this natively.

3. **Wrapper structs** (the (C) approach) embed one or more predicate structs alongside value field(s). A typed accessor method exposes the value (`Int() int`, `String() string`, etc.) so the wrapper can participate in generic-constrained signatures.

4. **Function signatures** pick one of three shapes depending on how much caller flexibility the function wants:
   - Concrete wrapper type (strict, zero-cost, the default).
   - Generic with interface constraint (flexible, zero-cost).
   - Plain interface value (flexible, with interface-value boxing cost).

All three coexist. Callers see plain Go; `gopls` is happy.

## Walk-through

### Predicate markers

```go
package proven

type Positive struct{}

func (Positive) Check(x int) bool { return x > 0 }
func (Positive) _isPositive()     {}

type IsPositive interface{ _isPositive() }
```

Any struct embedding `Positive` gains `_isPositive()` by promotion and therefore satisfies `IsPositive`.

### Wrappers

Primitive wrappers shipped by the `proven` package:

```go
type PositiveInt struct {
    Positive
    V int
}
func (p PositiveInt) Int() int { return p.V }

type NonEmptyString struct {
    NonEmpty
    S string
}
func (n NonEmptyString) String() string { return n.S }
```

Domain wrappers follow the same pattern:

```go
type UserID struct {
    proven.Positive
    V int
}
func (u UserID) Int() int { return u.V }

type Amount struct {
    proven.Positive
    V    int
    Curr Currency
}
func (a Amount) Int() int { return a.V }

type Currency struct {
    proven.ValidCurrency
    S string
}
func (c Currency) String() string { return c.S }
```

Embedding a predicate is the only way to make a struct "carry a proof," and that embedding is exactly what the preprocessor verifies at construction sites.

### Function signatures — three shapes

**Concrete (default).** Parameters are exactly the wrapper type the function needs.

```go
func Transfer(from UserID, to UserID, amount Amount) error { /* ... */ }

Transfer(
    UserID{V: 1},
    UserID{V: 2},
    Amount{V: 100, Curr: Currency{S: "USD"}},
)
```

Strict — callers construct exactly the expected type. Zero runtime cost.

**Generic with interface constraint (zero-cost polymorphism).**

```go
type IntValue interface{ Int() int }

func LogPositive[T interface{ proven.IsPositive; IntValue }](x T) {
    fmt.Println(x.Int())
}

LogPositive(UserID{V: 1})     // Go infers T = UserID
LogPositive(PositiveInt{V: 7}) // Go infers T = PositiveInt
```

No boxing — `T` is instantiated to the concrete type at each call. Useful when a function should accept any wrapper bearing a particular proof (e.g., a generic logger, validator, or formatter).

**Plain interface value (flexible, with boxing).**

```go
type PositiveIntBearer interface {
    proven.IsPositive
    Int() int
}

func LogPositive(x PositiveIntBearer) {
    fmt.Println(x.Int())
}
```

One interface-value allocation per call (escape-to-heap in most cases). Appropriate for non-hot paths, APIs crossing package boundaries where generic constraints would be awkward, or collections of heterogeneous wrappers.

### Composition

Embed multiple predicates in one wrapper:

```go
type Note struct {
    proven.NonEmpty
    proven.MaxLen280
    S string
}
func (n Note) String() string { return n.S }
```

`Note` satisfies both `IsNonEmpty` and `IsMaxLen280` by promotion. Functions requiring either interface (or both) accept it.

### Subsumption — free

Go's structural typing handles subsumption automatically:

```go
// Log requires only NonEmpty:
type StringValue interface{ String() string }

func Log[T interface{ proven.IsNonEmpty; StringValue }](x T) { /* ... */ }

// Note carries NonEmpty + MaxLen280:
n := Note{S: "hello"}
Log(n) // fine — Note satisfies IsNonEmpty
```

No `Weaken`, no preprocessor-rewritten cast, no subsumption-algebra code in the preprocessor. This is the reason for the pivot — subsumption was the single biggest source of IDE friction, and moving to interface-based signatures makes it Go's problem, not ours.

Nominal distinctions still hold where we want them: a `PremiumUserID` does not silently become a `UserID` unless the two share a common interface that abstracts the distinction. Subsumption is opt-in at the interface level, not automatic between struct types.

### Method ambiguity

When two embedded predicates both define `Check`, accessing `Check` on the composite is ambiguous:

```go
n := Note{S: "hi"}
// n.Check(n.S) — compile error: ambiguous selector
n.NonEmpty.Check(n.S)  // ok
n.MaxLen280.Check(n.S) // ok
```

This is fine. User code does not call `Check` on composites; the preprocessor calls each predicate's `Check` individually when generating runtime guards for `TrustMe`-equivalent boundary paths.

## What the preprocessor actually does

With subsumption and inference delegated to Go, the preprocessor's remaining responsibilities are narrow:

1. **Verify proof obligations at construction sites.** When the user writes `UserID{V: -5}`, verify that `-5` satisfies `Positive`. Flow-sensitive analysis over preceding conditionals. Fail the build when the proof cannot be discharged.
2. **Boundary guards.** At explicit boundary sites (to be designed — a likely shape is an opt-in `proven.Trust` wrapper or a struct-tag annotation on deserialization targets), inject `Check` calls that panic on violation.
3. **`Const` evaluation.** Compute pure expressions at build time and substitute literals.

Proof-expression algebra, `Weaken`, `And`/`Or`/`Not`, and the `Refined[P, T]` wrapper all disappear from the hot path of the design.

## Tradeoffs

**Gains.**

- gopls-native. No red squiggles on subsumption. No generic-inference failures. The IDE sees plain Go everywhere.
- Go-idiomatic. Interfaces for flexibility, concrete types for strictness — exactly how Go already encourages API design.
- Typo-safe. A misspelled predicate is a Go compile error, not a silent skipped validation.
- Preprocessor is smaller. Subsumption and inference are Go's job, not ours.

**Costs.**

- Method ceremony. Each predicate needs a one-line marker method. Each wrapper needs a typed accessor method. Mechanical, but real.
- Composite method ambiguity on shared names (`Check`). Access via the embedded field rather than the composite.
- Modest boilerplate per wrapper type. A small code generator could absorb this if it proves noisy; defer until demonstrable.

## Deprecated

The following API surface from the A+C era is no longer the primary path:

- `proven.Refined[P, T]`, `proven.Attest[P]`, `proven.TrustMe[P]`.
- `proven.And`, `proven.Or`, `proven.Not`.
- The `Weaken` cast planned in `docs/ide-integration.md`.

These were useful transitional names; they did not survive the empirical IDE friction. They can remain in `pkg/proven` as a fallback for truly ad-hoc one-off primitive constraints, but new code should prefer the embedding pattern documented above.

## What's next

1. Migrate the predicate-and-wrapper vocabulary into `pkg/proven` with the marker-method convention.
2. Rewrite `example/basic` to use the new pattern end-to-end.
3. Keep `internal/inferenceexperiment` and `internal/embeddingexperiment` as regression tests — they document Go behavior we rely on.
4. Start the preprocessor skeleton: package scan + AST walk for struct-literal construction sites, with a trivial predicate-for-literal checker as the first pass.
