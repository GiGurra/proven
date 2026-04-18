# Design: runtime-degrading contracts, statically discharged

This is the authoritative design for proven. Earlier design iterations — generic wrapper types (`Refined[P, T]`), struct embedding (the (C) approach), interface-based signatures (the structural approach), and non-generic type aliases — were explored and rejected in favor of this one. Each is documented separately and marked superseded.

## The pattern

Functions declare preconditions on their parameters and postconditions on their return values by calling `proven.That` and `proven.Returns` inside the function body. The signature stays pure Go. Predicates are ordinary functions of type `func(T) bool`.

```go
func isPositive(x int) bool    { return x > 0 }
func isNonEmpty(s string) bool { return len(s) > 0 }

func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty)
    // ... body ...
    return nil
}
```

The package's execution model is shaped by one strict goal: **the IDE experience must be green, but accidentally building without the preprocessor must fail loudly**. The implementation gates proven at *link time*, not compile time, via a single helper named `atCompileTime`.

`That` and `Returns` wrap their checks in an `atCompileTime(func() { ... })` block. `atCompileTime` is declared via `//go:linkname` to an external symbol (`_proven_atCompileTime`) with no Go body. The preprocessor supplies that symbol during its toolexec pass.

- **Type-checking is always valid.** `gopls`, IDEs, and `go vet` see ordinary Go — no build tags, no editor configuration, no squiggles.
- **Link fails without the preprocessor.** `go build` / `go test` on any `main` or test target produces:
  ```
  relocation target _proven_atCompileTime not defined
  ```
  Pure library builds (`go build ./...` on non-main packages) still produce `.a` archives, but any downstream binary refuses to link.
- **With the preprocessor** (`GOFLAGS="-toolexec=proven"`), the toolexec pass injects the symbol, runs the static discharge, erases proven call sites whose obligations are discharged, and fails the build when they are not.

Inside an `atCompileTime` block, the code is material the preprocessor reads; it never executes at runtime. `That`'s block contains a loop evaluating each predicate (`_ = pred(v)`) purely as a structural hint for the reader. There is no runtime panic path, no runtime cost; the runtime bodies are wholly inert on any path that reaches them (and in a correctly built binary, no path does).

## The API surface

```go
func That[T any]   (v T, preds ...func(T) bool)
func Returns[T any](v T, preds ...func(T) bool) T

func All[T any](preds ...func(T) bool) func(T) bool
func Any[T any](preds ...func(T) bool) func(T) bool
func Not[T any](p func(T) bool)        func(T) bool
```

That's it. No type parameters to wrangle at call sites, no marker interfaces, no combinator type hierarchies, no wrapper types to construct.

`That` and `Returns` are variadic: multiple predicates AND-compose. For OR composition or reusable composite values, wrap with `All` / `Any` / `Not`:

```go
var smallPositive = proven.All(isPositive, lessThan100)

func setPercent(p int) {
    proven.That(p, smallPositive)
}
```

## How callers discharge obligations

The preprocessor accepts these discharge patterns in the caller:

- **Literal analysis.** `Transfer(42, "hi")` — `42 > 0` and `len("hi") > 0` are proven by constant evaluation.
- **Preceding predicate check.** `if isPositive(x) { Transfer(x, ...) }` — inside the then-branch, `isPositive(x)` is a fact.
- **Early-return guard.** `if !isPositive(x) { return }; Transfer(x, ...)` — after the guarded return, `isPositive(x)` is established.
- **Conjoined guards.** `if isPositive(x) && x < 100 { ... }` — both clauses become facts in the then-branch.
- **Flow from `Returns`.** Result of a function whose body declares `proven.Returns(v, isPositive)` flows into a matching `That` check without re-proof.
- **Explicit trust (planned).** A future `proven.Trust(x, pred)` variant accepts the obligation as a deliberate runtime guard — useful at boundaries (deserialization, HTTP handlers) where static proof is impossible.

Patterns not supported in v1:
- Interprocedural flow into arbitrary helper functions.
- Proof through complex argument expressions (`That(transform(x), p)` is v2).
- Proofs that require SMT — inequalities chained transitively, user-declared predicate implications, etc.

## Why this design over the alternatives

**Generic wrapper types (`Refined[P, T]`).** Rejected because Go 1.26 cannot infer phantom type parameters from call-site context. Users would have to write `proven.Attest[Positive](x)` at every call, plus a separate `Weaken` cast for every subsumption boundary. The IDE story was also poor — gopls flags subsumption as type errors. Captured in [`parameter-constraint-syntax.md`](parameter-constraint-syntax.md) and [`ide-integration.md`](ide-integration.md).

**Struct embedding ((C)).** Viable and IDE-native, but every constrained primitive needs a wrapper struct with an accessor method. Heavyweight for ad-hoc constraints. Composition works via embedding but distinct struct types don't subsume (nominal typing), forcing explicit narrowing at boundaries.

**Structural interfaces ((B)).** The best IDE experience of the type-based options — gopls handles subsumption natively via interface embedding — but interface values involve boxing and escape-to-heap, plus a combinatorial explosion of generated wrapper types per proof-set × underlying-type combination.

**Non-generic type aliases.** `type PositiveInt = int` works and is fully transparent, but requires declaring every (predicate-set × primitive) combination as a named alias. Scales poorly with vocabulary.

**Doc-comment annotations.** Typo-unsafe. A misspelled `// proven:requires` silently skips validation.

The `That`-in-body design avoids every one of these pitfalls:

- **Signatures are plain Go.** `gopls` has nothing special to cope with. No inference failures, no red squiggles, no ceremony.
- **Predicates are plain functions.** Typos are Go compile errors. No marker interfaces or wrapper hierarchies.
- **Linker gate.** Forgetting the preprocessor fails the link step with a clear diagnostic; it cannot silently degrade to runtime-only checking. IDE-level type checking stays green regardless, so in-editor flow is unaffected.
- **Preprocessor scope shrinks.** No subsumption algebra, no type-level proof propagation, no wrapper-type synthesis. The preprocessor is a flow-sensitive analyzer + erasing rewriter. Substantially simpler than prior designs.

## The known downside

Preconditions are not visible in the function signature. A reader of `func Foo(i int)` has no signature-level cue that `Foo` requires `i > 0`. They must open the body (or read godoc).

Mitigations:

- **Godoc convention.** Document preconditions in the function's doc comment: `// Foo requires isPositive(i).` The preprocessor could auto-generate godoc entries from `That` calls; worth considering.
- **Editor hover extension.** A small gopls plugin or a sibling LSP could surface "this function asserts …" on hover. Tractable later if the ergonomic pain shows up.
- **Accept the tradeoff.** Most Go APIs don't document preconditions in the signature either (`json.Unmarshal` takes `[]byte`, no hint about validity). The precondition pattern isn't newly invisible — it's just not made visible. This is the cost of refusing to touch the Go type system.

## What the preprocessor actually does

1. **Scan bodies for `proven.That` and `proven.Returns`.** Build a per-function summary: parameters each have a list of predicate functions that must hold; returns may be annotated with postcondition predicates.
2. **Flow-sensitive analysis in each caller.** Collect facts from conditionals, guards, prior `Returns` postconditions, and literal evaluation. For each call to an annotated function, discharge every predicate on every corresponding argument.
3. **Emit or fail.** When all obligations discharge, rewrite the callee's body (for this build) to erase the `That`/`Returns` calls. When they don't, fail the build with a diagnostic naming the unproven predicate and the call site.
4. **`Const` evaluation.** Compute pure expressions at build time and substitute literals. Orthogonal to the contract story but part of the same preprocessor pipeline.

No proof-expression subsumption, no marker-interface synthesis, no wrapper-type generation, no phantom-parameter tricks.

## Open questions

- **Boundary guards.** Likely a `proven.Trust(v, preds...)` variant that wraps a runtime check and carries the proof forward. Name and exact semantics TBD.
- **Cross-package obligations.** When package A calls package B's annotated function, A's preprocessor needs B's function summary. Either re-scan B's source (slow) or emit per-package sidecar summary files during B's build (fast, rewire-style). Lean toward the sidecar.
- **Predicate identity.** Predicates are identified by function declaration (package + function name). A predicate with the same name but defined in a different package is a distinct obligation. Users who want shared predicates import them from a single place. This is simple and matches Go's usual handling of function identity.
- **Argument expressions.** v1 supports direct parameter references (`That(amount, p)`) and the result of a `Returns`-annotated call. Arbitrary expressions (`That(transform(x), p)`) wait for v2.
- **Godoc integration.** Auto-emit preconditions into the function's godoc entry during the preprocessor pass? Nice-to-have; low priority until v1 is proven useful.

## What's next

1. `pkg/proven/proven.go` already carries the runtime-checkable `That` / `Returns` / `All` / `Any` / `Not` implementations.
2. `example/basic/` demonstrates the pattern end-to-end.
3. The preprocessor skeleton is still to build: toolexec entry, per-package scan, flow analyzer, diagnostic emission. The scan step can follow rewire's shape closely.
4. The `internal/*experiment` packages stay as regression documents for the Go-language behavior each rejected design relied on.
