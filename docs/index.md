# Proven

[![CI Status](https://github.com/GiGurra/proven/actions/workflows/ci.yml/badge.svg)](https://github.com/GiGurra/proven/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GiGurra/proven)](https://goreportcard.com/report/github.com/GiGurra/proven)

Compile-time contracts for Go. Declare what must hold inside a function body using plain `func(T) bool` predicates; the preprocessor walks the call graph at build time, accepts every call whose preconditions it can prove, and fails the build on the ones it can't.

```go
// Declare a precondition inside the function body:
func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty, maxLen280)
    // ... body ...
}

// Callers see the facts the body proves on the returned identifier —
// no explicit postcondition call required:
func Normalize(x int) int {
    proven.That(x, isPositive)
    return x // callers get isPositive as a fact on the result
}

// proven.Returns pins the postcondition to the declaration site.
// The fact already flows to callers without it — what Returns adds
// is compile-time verification HERE, so a future edit that drops
// the proof from the body breaks this function's build instead of
// silently withdrawing the claim at every caller:
func Clamp(x int) int {
    proven.That(x, isPositive)
    return proven.Returns(x, isPositive)
}

// For a literal or computed expression the analyzer can't reason about,
// trust.Returns advertises the postcondition without a runtime check:
func DefaultUserID() int {
    return trust.Returns(42, isPositive) // programmer: "42 is obviously positive"
}

// Declare an implication once; the preprocessor uses it to prove preconditions:
var _ = infer.From(isSmallPositive).To(isPositive)

// Boundary validators establish facts on raw input:
// - prove.That(raw, pred) returns an error (handler path)
// - prove.Must(raw, pred) panics on failure (startup path)
// - trust.That(raw, pred) skips the runtime check (programmer's word)

// Put it together: validate once at the boundary, then every
// downstream precondition is proven at compile time.
func main() {
    amount, err := prove.That(readAmount(), isPositive)
    if err != nil {
        log.Fatal(err)
    }
    note := trust.That("hello", isNonEmpty, maxLen280)

    if err := Transfer(amount, note); err != nil {
        log.Fatal(err)
    }
    _ = Clamp(Normalize(amount)) // isPositive flows through the nested chain
}
```

Signatures stay plain Go. Predicates are ordinary functions. No wrapper types, no generics ceremony, no codegen. `gopls` and `go vet` see ordinary code, so the IDE is always green — but building without the preprocessor fails loudly at link time, so you cannot silently ship code that bypasses the contract system.

**The preconditions of every function become visible, machine-checked documentation** — executable docstrings that the compiler enforces at every call site.

## Why you might want this

Systems grow. Past a certain size it becomes effectively impossible to remember what invariants were imposed where, or why. Engineers re-validate defensively, trust assumptions that have drifted, or simply forget to check. The cost is runtime bugs, redundant validation at every layer, and a maintenance burden that scales faster than the codebase.

A `proven.That(x, isPositive)` at the top of a function body does two jobs at once. It's **documentation** the reader can't miss — the function's requirements live in its body, not in a comment that may have drifted from reality. And it's a **check** the compiler runs at every call site, at build time, at zero runtime cost once the proof is established. A predicate written once is enforced everywhere the function is called, for as long as it stays declared.

Proven shifts that burden onto the compiler. A function declares once what must hold; the preprocessor proves it at every call site. If the proof succeeds, nothing runs at runtime. If any path can't be proved, the build fails with a diagnostic pointing at the offending call site.

### Nil safety

`pkg/proven` ships a generic `NonNil[T any](p *T) bool`. Declare the precondition once; the preprocessor reads the ordinary Go nil-safety idioms and enforces non-nilness at every call site:

```go
func greet(u *User) {
    proven.That(u, proven.NonNil)
    _ = u.Name // safe — greet's input is non-nil by contract
}

// Plain Go nil-compare in a guard.
func byCompare(u *User) {
    if u != nil {
        greet(u) // proven.NonNil(u) holds in the then-branch
    }
}

// Early-return nil-bail — u is known non-nil for the rest of the function.
func byEarlyReturn(u *User) {
    if u == nil {
        return
    }
    greet(u)
}

// Boundary validation — runtime nil-check, fact carries forward on err==nil.
func byBoundary(id int) {
    u, err := prove.That(lookupUser(id), proven.NonNil)
    if err != nil {
        return
    }
    greet(u)
}

// Known-non-nil literal expressions accepted at build time with no
// runtime check — &T{...}, new(T), make(...) are never nil.
greet(&User{ID: 1})
greet(new(User))
```

No separate `NonNil[*User]` type, no wrapper-based newtype dance — just one predicate, enforced everywhere a pointer flows.

### Library predicates and compile-time literal evaluation

`pkg/proven` also ships `Positive`, `Negative`, `NonNegative`, `NonPositive`, `Zero`, `NonZero`, `Even`, `Odd`, `NonEmpty`, `Empty`, and `Nil` alongside `NonNil`. When a caller passes a literal (integer, float, string) or a simple compile-time expression (`nil`, `&T{...}`, `new(T)`, `make(...)`) to a function whose precondition is one of these library predicates, the preprocessor evaluates the predicate at build time and accepts the call with no runtime check.

Scope is deliberately narrow — only library-included predicates, only simple argument shapes, and package-level `const` references are not yet resolved. Future work may loosen these limits, potentially allowing some form of user-supplied compile-time inference.

## Honest limits — mutation and soundness

Facts about a variable reflect what the analyzer knows *at a program point*. Because Go has no immutability markers, any mutation invalidates prior facts. The preprocessor catches what it can syntactically and is explicit about the rest.

**Invalidated automatically.** Reassigning `x`, `x++` / `x--`, compound assigns (`x += 1`), field / index writes (`x.a = ...`, `x[i] = ...`), and taking `&x` to pass it to a function all drop every fact on `x`. Downstream calls that needed those facts fail the build until you re-prove them (guard, `prove.Must`, `trust.That`, etc.):

```go
if proven.Positive(x) {
    x = -1        // facts on x forgotten
    target(x)    // cannot prove proven.Positive — build fails
}

if proven.Positive(x) {
    mutate(&x)   // &x escapes; x's facts forgotten (callee might rewrite *x)
    target(x)    // cannot prove — build fails
}
```

**Known soundness gap.** A call like `mutateNestedStruct(root)` that reaches into a shared sub-object (through a pointer field, a slice, a map, a channel) can mutate state the analyzer has facts on without us seeing it. Without full escape / type analysis this is invisible, and the facts on `root` persist after the call.

The honest advice: prove what you need, hand the value to the next call that needs the proof, and don't route it through functions that might mutate it in ways the analyzer can't trace. Functions meant to preserve a predicate should re-declare it as their own `proven.That` precondition — the chain stays visible at every hop, and the caller proves the predicate anew at each site where the fact might have become stale.

## How it compares

Compile-time contracts aren't new — Eiffel's Design by Contract, Ada `Pre`/`Post`, C++ concepts, Rust's type-state patterns are all takes on the same idea. Their shared failure mode is that a requirement verified only at compile time is fragile: nothing catches you accidentally removing it, weakening it, or forgetting to state it.

Proven addresses this by letting you *test* at runtime that the declarations are what you think. The preprocessor erases `proven.That` at build as usual, but inside tests you can opt in to running the blocks at runtime and assert that the right predicate fires on the right parameter. Silent drift between assertion and implementation turns into a test failure.

## Pick your path

- **[Getting Started](getting-started.md)** — install, configure, first passing build.
- **[Design](design.md)** — authoritative design doc: the pattern, the API, the rationale for this shape over earlier alternatives.
- **[Companion Packages](companion-packages.md)** — how `proven`, `prove`, `trust`, `infer`, `infertest` cooperate.
- **[Relations Between Values](relations.md)** — the tuple-subject pattern for multi-value relations, plus the explored-and-deferred alternatives (currying, explicit `proven.Relation`).
- **[Roadmap](todo/roadmap.md)** — what's done, what's next, what's out of scope.
- **[Compile-time Evaluation](comptime.md)** — the Zig-style comptime exploration that lived briefly on the roadmap as `infer.Const`, concluded out of scope.

## Status

Experimental — APIs and internals may change. See the [roadmap](todo/roadmap.md) for the current state of the preprocessor pipeline and what's next.
