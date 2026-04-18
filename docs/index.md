# Proven

[![CI Status](https://github.com/GiGurra/proven/actions/workflows/ci.yml/badge.svg)](https://github.com/GiGurra/proven/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GiGurra/proven)](https://goreportcard.com/report/github.com/GiGurra/proven)

Compile-time contracts for Go. Declare what must hold inside a function body using plain `func(T) bool` predicates; the preprocessor walks the call graph at build time, discharges obligations it can prove, and fails the build on the ones it can't.

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

// Declare an implication once; the preprocessor uses it to discharge obligations:
var _ = infer.From(isSmallPositive).To(isPositive)

// Boundary validators establish facts on raw input:
// - prove.That(raw, pred) returns an error (handler path)
// - prove.Must(raw, pred) panics on failure (startup path)
// - trust.That(raw, pred) skips the runtime check (programmer's word)

// Put it together: validate once at the boundary, then every
// downstream precondition discharges at compile time.
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

**The point isn't more validation. It's to stop having to remember.**

## Why you might want this

Systems grow. Past a certain size it becomes effectively impossible to remember what invariants were imposed where, or why. Engineers re-validate defensively, trust assumptions that have drifted, or simply forget to check. The cost is runtime bugs, redundant validation at every layer, and a maintenance burden that scales faster than the codebase.

Proven shifts that burden onto the compiler. A function declares once what must hold; the preprocessor proves it at every call site. If the proof succeeds, nothing runs at runtime. If any path can't be proved, the build fails with a diagnostic pointing at the offending call site.

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
