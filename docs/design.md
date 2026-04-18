# Design: compile-time contracts with a link-time gate

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

Inside an `atCompileTime` block, the code is material the preprocessor reads. In production builds the block never executes — the preprocessor has either erased the call site or substituted a compile error. In the default test configuration (via `proventest` — see below) the block is also a no-op, so runtime cost is zero.

Tests can opt into running the block at runtime for wiring verification. `proventest.WithChecks(fn)` flips a flag for the duration of `fn`; inside that window, each `atCompileTime` block actually executes its closure, and a failing predicate panics with a `proven.Violation` naming the failing predicate. The high-level helper `proventest.AssertFails(t, pred, fn)` runs `fn` under `WithChecks` and asserts that a specific predicate is what fires — catching drift between the assertion and the implementation without needing the preprocessor to be built yet.

## Package layout

- `pkg/proven/` — the public API: `That`, `Returns`, `And`, `Or`, `Not`. Plus `Violation` (panic value under `proventest.WithChecks`) and `PredicateName` (runtime function name resolution for diagnostics).
- `pkg/proventest/` — test-only support: supplies the `_proven_atCompileTime` symbol so test binaries link without the preprocessor, and exposes `WithChecks`, `AssertFails`, `AssertAnyFailure` for opt-in runtime verification. Never imported from production code.
- `pkg/infer/` — fluent builder for declaring predicate implication rules. `infer.From(p...).To(q...)` (unconditional) or `infer.From(p...).Given(c...).To(q...)` (conditional). Every slot is variadic and AND-composes, same as `proven.That` / `prove.That` / `trust.That`. Consumed by the preprocessor to discharge obligations by subsumption.

## The API surface

```go
func That[T any]   (v T, preds ...func(T) bool)
func Returns[T any](v T, preds ...func(T) bool) T

func And[T any](preds ...func(T) bool) func(T) bool
func Or[T any](preds ...func(T) bool) func(T) bool
func Not[T any](p func(T) bool)        func(T) bool
```

That's it. No type parameters to wrangle at call sites, no marker interfaces, no combinator type hierarchies, no wrapper types to construct.

`That` and `Returns` are variadic: multiple predicates AND-compose. For OR composition or reusable composite values, wrap with `And` / `Or` / `Not`:

```go
var smallPositive = proven.And(isPositive, lessThan100)

func setPercent(p int) {
    proven.That(p, smallPositive)
}
```

## How callers discharge obligations

The preprocessor accepts these discharge patterns in the caller. All of them produce the same kind of fact in the analyzer — `{predicate: P, var: x}` — and any one of them discharges a `proven.That(x, P)` downstream. Pick whichever matches the shape of the surrounding code.

- **Preceding predicate check.** `if isPositive(x) { Transfer(x, ...) }` — inside the then-branch, `isPositive(x)` is a fact.
- **Early-return guard.** `if !isPositive(x) { return }; Transfer(x, ...)` — after the guarded return, `isPositive(x)` is established for the rest of the enclosing block.
- **Conjoined guards.** `if isPositive(x) && x < 100 { ... }` — both clauses become facts in the then-branch.
- **The function's own declared preconditions.** A function that declares `proven.That(x, isPositive)` at the top of its body starts analysis with `isPositive(x)` already a fact — every caller has proved this at the call site, so inside the function the predicate holds by construction. This lets a function forward a parameter to another function with the same precondition without a redundant guard, and it is the mechanism that makes `return proven.Returns(x, isPositive)` verify against the function's own precondition.
- **Flow from `proven.Returns`.** Result of a function whose body declares `proven.Returns(v, isPositive)` flows into a matching `That` check without re-proof. The postcondition rides along via the Phase 6 cross-package sidecar, so it works across module boundaries. The `proven.Returns` site itself is verified: the value argument must be an identifier and each predicate must already be a flow-state fact at that site, so the callee cannot advertise an unproven postcondition.
- **Boundary validation with [`prove`](companion-packages.md).** At an external boundary (HTTP body, decoded JSON, CLI arg, database row) static proof is impossible. `prove.That(raw, preds...) (T, error)` runs a runtime check and, on the `err == nil` branch, establishes each supplied predicate as a fact on the returned value. `prove.Must(raw, preds...) T` is the panic-on-fail variant for startup paths where failure is fatal. Both are real runtime guards that carry the proof forward into the remainder of the function.
- **Local fact injection with [`trust`](companion-packages.md) — the "trust me" escape hatch.** Every other discharge path in proven gets the compiler (or a runtime check) behind the claim. `trust.That(v, preds...) T` is the one call site where *you* stand behind it instead: no runtime check, no static proof, the fact is accepted on your word and your git blame is the evidence. The preprocessor erases the call and treats each listed predicate as a fact on the LHS. Use when a value has already been validated by a mechanism the analyzer cannot see (external schema validator, prior audited invariant, DB CHECK constraint). Reach for `prove.Must` first if a free runtime check would do — `trust.That` earns its keep only when re-validation would duplicate a known-correct earlier check and the cost is meaningful. `trust.Returns(v, preds...) T` combines trust.That's honor-system semantics with `proven.Returns`'s postcondition advertisement: the fact is asserted without a runtime check AND propagated to every caller via the function's summary. It is the cleanest shape for "my literal/expression trivially satisfies the predicate; take my word and advertise it as the function's postcondition."
- **Declared implication.** When the caller has established predicate `P` and the callee requires `Q`, the preprocessor discharges `Q` if the module declares `infer.From(P).To(Q)` (optionally with `.Given(ctx)`). Implications are **trusted** — no symbolic verification — but `infertest.Verify(t, rule, samples...)` lets you property-test a rule at test time to catch declared-but-false implications.

Patterns not supported in v1 — the preprocessor **fails the build** with a Go-standard `file:line:col:` diagnostic rather than silently dropping the obligation:

- **Unnamed predicates.** Every predicate argument to `proven.That`, `proven.Returns`, `prove.That`, `prove.Must`, `trust.That`, and each slot in `infer.From(...).[Given(...).]To(...)` must be a named function or a `pkg.Name` selector. Function literals and inline combinator calls (`proven.And(a, b)` passed directly to `That`) fail the build; assign them to a package-level `var` and reference the name. This is the "no silent bypass" principle in the scanner: a predicate the scanner cannot track by identity cannot be used for cross-package discharge, and accepting it would silently weaken the contract.
- **Non-parameter subjects.** The first argument to `proven.That` must be an identifier that is a parameter of the enclosing function. Computed subjects (`proven.That(foo.bar, p)`, `proven.That(compute(x), p)`) and non-parameter locals fail the build: they have no position in the callee's signature that callers could be required to discharge.
- **Literal analysis.** `proven.That(42, isPositive)` does not constant-fold in the scanner — wrap the value in a guard, a `prove.Must`, or a `trust.That`.
- **Interprocedural flow into arbitrary helper functions** without a `proven.That` precondition declared by the helper itself.
- **Proof through complex argument expressions** (`That(transform(x), p)` is v2).
- **Symbolic predicate reasoning beyond declared `infer` rules** (full SMT remains out of scope).

## Relations between values

Predicates are `func(T) bool` — strictly unary. A relation over multiple values ("controller `c` has authorized executor `e`", "session `s` is owned by user `u`") is expressed by packing the participating subjects into a domain struct and writing the predicate as unary over that struct:

```go
type AuthCtx struct {
    S Session
    U User
    R Resource
}

func canModify(a AuthCtx) bool { /* ... */ }

func modifyResource(a AuthCtx) {
    proven.That(a, canModify)
}
```

Every existing discharge pattern — guards, early-return, `proven.Returns`, `prove.That`/`Must`, `trust.That`, `infer.From/Given/To`, the cross-package sidecar, the erasure rewriter — applies to tuple subjects unchanged. No new API, no new `Fact` shape. The cost is that users must name their relations as domain types, which tends to be good design but is a new discipline.

Currying (`f(s, u)(r)` as a nested unary predicate) and an explicit `proven.Relation(pred, subjects...)` API were both explored and deferred. See [`relations.md`](relations.md) for the full exploration, AST shapes for each approach, and the triggers that would reopen the decision.

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
2. **Scan for `infer.From(...).To(...)` declarations.** Populate an implication graph: `P ⇒ Q` (optionally under context `C`).
3. **Flow-sensitive analysis in each caller.** Collect facts from conditionals, guards, prior `Returns` postconditions, and literal evaluation. For each call to an annotated function, discharge every predicate on every corresponding argument, using implication-graph walks when a direct match fails.
4. **Emit or fail.** When all obligations discharge, rewrite the callee's body for this build to erase the `That` / `Returns` calls. When they don't, fail the build with a diagnostic naming the unproven predicate and the call site.

No marker-interface synthesis, no wrapper-type generation, no phantom-parameter tricks. The implication graph from step 2 is the only subsumption-like machinery, and it is purely structural — a set of declared-and-trusted implications, not a solver.

Zig-style compile-time evaluation of pure expressions — historically lumped under this project as `infer.Const` — was explored and declared out of scope; the mechanisms and use cases share little with contract discharge, and bundling them obscures both. See [`comptime.md`](comptime.md).

## Open questions

- **Boundary guards.** Likely a `proven.Trust(v, preds...)` variant that wraps a runtime check and carries the proof forward. Name and exact semantics TBD.
- **Cross-package obligations.** When package A calls package B's annotated function, A's preprocessor needs B's function summary. Either re-scan B's source (slow) or emit per-package sidecar summary files during B's build (fast, rewire-style). Lean toward the sidecar.
- **Predicate identity.** Predicates are identified by function declaration (package + function name). A predicate with the same name but defined in a different package is a distinct obligation. Users who want shared predicates import them from a single place. This is simple and matches Go's usual handling of function identity.
- **Argument expressions.** v1 supports direct parameter references (`That(amount, p)`) and the result of a `Returns`-annotated call. Arbitrary expressions (`That(transform(x), p)`) wait for v2.
- **Godoc integration.** Auto-emit preconditions into the function's godoc entry during the preprocessor pass? Nice-to-have; low priority until v1 is proven useful.

## What's next

1. `pkg/proven/proven.go` carries the runtime-checkable `That` / `Returns` / `And` / `Or` / `Not` plus the `atCompileTime` linker gate.
2. `pkg/proventest/` provides the linker stub for test binaries and the `WithChecks` / `AssertFails` helpers.
3. `pkg/infer/infer.go` provides the fluent inference-rule builder.
4. `example/basic/` demonstrates the pattern end-to-end with `TestWiring_*` tests verifying the assertions are wired correctly.
5. **Preprocessor is next.** Toolexec entry, per-package scan (functions with `That` / `Returns` + `infer.From(...).To(...)` declarations), flow-sensitive analyzer with implication-graph lookup, AST rewriter that erases discharged calls and supplies the `_proven_atCompileTime` symbol, diagnostic emission. The scan and toolexec plumbing can follow `rewire`'s shape closely; flow analysis is the novel part.
