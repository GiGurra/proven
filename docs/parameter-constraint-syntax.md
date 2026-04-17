# Parameter constraint syntax — alternatives considered

> **Status: superseded.** The A+C decision recorded at the bottom of this
> file has been overturned. The current design uses interface-based proof
> signatures plus struct-embedded proof markers — see [`design.md`](design.md).
> This file is kept as historical analysis of the options considered.

---


How do you attach "this value has proof P" to a Go function parameter, without extending Go's grammar? Go's type system is nominal for struct types and structural for interfaces. Primitives (`int`, `string`, …) cannot carry methods. Generics support phantom type parameters but not value-dependent types. These constraints shape every option below.

This document captures the three approaches we considered, the tradeoffs, and the chosen direction. Revisit before adding a fourth approach or before changing the public API shape.

---

## Approach A — Nominal wrapper: `Refined[P, T]`

A single generic wrapper type carries the proof in its type parameter.

```go
// Predicate types are zero-size structs. The Check method is optional
// (only needed for the TrustMe runtime-guard path).
type Positive    struct{}
type LessThan100 struct{}

func (Positive)    Check(x int) bool { return x > 0 }
func (LessThan100) Check(x int) bool { return x < 100 }

func SetPercent(p proven.Refined[proven.And[Positive, LessThan100], int]) {
    v := p.Unwrap()
    _ = v
}

// Call site:
SetPercent(proven.Attest[proven.And[Positive, LessThan100]](42))
```

**Properties**

- Works directly on primitives. No requirement that the value type owns methods.
- Zero runtime cost when the proof succeeds — `Refined[P, T]` is a single-field struct around `T`.
- Composition requires combinators (`And`, `Or`, `Not`) or user-defined compound predicate types (`type SmallPositive struct{}` with a combined `Check`).
- Proof is nominal: `Refined[Positive, int]` and `Refined[NonNegative, int]` are unrelated types even if `Positive` implies `NonNegative` logically. Subtyping between predicates is extra work for the checker.

---

## Approach B — Structural interfaces with marker methods

Each predicate is an interface with an unexported marker method. Composition happens through Go's native interface embedding.

```go
type Positive[T any]    interface { Unwrap() T; _provenPositive() }
type LessThan100[T any] interface { Unwrap() T; _provenLessThan100() }

func SetPercent(p interface {
    proven.Positive[int]
    proven.LessThan100[int]
}) {
    v := p.Unwrap()
    _ = v
}
```

For this to work, primitive values must be wrapped in a generated type that implements the marker methods:

```go
// Preprocessor-generated, hidden from users:
type _provenInt_posLt100 struct { v int }
func (_provenInt_posLt100) _provenPositive()    {}
func (_provenInt_posLt100) _provenLessThan100() {}
func (w _provenInt_posLt100) Unwrap() int       { return w.v }
```

**Properties**

- Composition is beautifully structural — listing required proofs in a parameter's interface is the Go-idiomatic way to compose capabilities.
- Cost: interface values involve boxing and an extra indirection. Inlining mitigates some of it, but the allocation story is worse than (A) and (C).
- The preprocessor has to generate a wrapper struct for **every unique combination of proofs × underlying type** that appears in the module. Binary bloat grows combinatorially with the predicate vocabulary.
- Values carry `Unwrap()` ceremony — no direct field access.

**Rejected.** Provides the worst of both worlds: structural composition (nice) but runtime boxing, generated wrappers you don't control, and mandatory `Unwrap()`.

---

## Approach C — Struct embedding with zero-size proof fields

Proof markers are zero-size structs. Domain types embed them alongside the underlying value(s). The proof literally rides in the value, at zero cost.

```go
type Percent struct {
    proven.Positive
    proven.LessThan100
    V int
}

func SetPercent(p Percent) { _ = p.V }

// Construction — under the preprocessor, the V field forces the proof check:
SetPercent(Percent{V: 42})
```

**Properties**

- Zero runtime overhead. Embedded zero-size fields do not change layout or size.
- Direct value access: `p.V` is just a field read.
- Composition is free via struct embedding — adding a proof is adding a line.
- The proof travels inside the value through function calls, returns, channels, map values — wherever the struct goes.
- `Positive{}` is always syntactically constructible. Enforcement is entirely the preprocessor's job: any `Percent{...}` literal must have the proof obligation discharged, and the zero-value `Percent{}` is only legal if the predicate holds on `int`'s zero value (which for `Positive` is false — so `var p Percent` is a build error).

**Tradeoff.** Users must declare a struct type per distinct parameter shape. For one-off primitives this is heavy; for domain entities (`UserID`, `Email`, `TransferAmount`) it's already what you'd write.

---

## Comparison

|                                         | A — `Refined[P, T]`          | B — Structural iface           | C — Struct embedding           |
| --------------------------------------- | ---------------------------- | ------------------------------ | ------------------------------ |
| Works on primitives directly            | yes                          | needs generated wrapper        | requires a struct wrapper      |
| Runtime cost when proof succeeds        | zero                         | boxing + indirection           | zero                           |
| Composition                             | combinators / compound types | native interface embedding     | native struct embedding        |
| Value access                            | `.Unwrap()`                  | `.Unwrap()`                    | direct field                   |
| Ceremony at parameter site              | moderate                     | low-medium                     | low (once the struct exists)   |
| Preprocessor-generated types per module | none                         | one per unique proof-set × T   | none                           |
| Good fit for                            | primitives, ad-hoc           | (nothing — see rejection)      | domain types that already exist |

---

## Decision

Use **A + C together**, not B.

- **A (`Refined[P, T]`)** is the default for primitives and for ad-hoc constraints on arguments that don't warrant a domain type. Example: `func f(x proven.Refined[Positive, int])`.
- **C (struct embedding)** is the default for domain entities — `UserID`, `Email`, `Amount`. The struct already exists; embedding proofs adds a line each. No `Unwrap()` tax, no combinator noise, and the proof composes with whatever else the domain type carries (IDs, timestamps, etc.).
- The two approaches share the same **predicate vocabulary** — a predicate is a zero-size struct type with an optional `Check` method. The same `Positive` type is usable as a type-parameter argument in (A) and as an embedded field in (C).

Doc-comment predicates (`// proven:where x > 0`) are a future layer on top. They expand into (A) at the preprocessor level.

## Open questions

- **Predicate subtyping.** If `SmallPositive` implies `Positive`, should `Refined[SmallPositive, int]` be assignable to a `Refined[Positive, int]` parameter? In Go's type system, no — they're distinct nominal types. The checker can special-case this at the call-site level, but propagation through generic functions is harder.
- **Proof erasure on unwrap.** In (C), `p.V` returns a raw `int`, losing the proof. That is intentional: once extracted, the value is "just an int" again and must be re-attested to re-enter a proven context. Worth revisiting if it creates friction in practice.
- **Generic predicate types.** `Positive` for `int` is easy; `Positive[T constraints.Ordered]` is more general but adds type-parameter noise everywhere it's used. Start with type-specific predicates; revisit if the duplication becomes painful.
- **Preprocessor's view of (C) construction.** Struct literals with field names, positional literals, `new(T)`, map lookups, and the zero value all create struct instances. Each is a proof obligation. The literal-vs-assignment coverage needs enumeration before implementation.
