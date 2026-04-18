# CLAUDE.md

## Project overview

`proven` is a compile-time contract system for Go, delivered as a `-toolexec` preprocessor. Functions declare preconditions and postconditions as ordinary runtime assertions inside the body (`proven.That`, `proven.Returns`). The preprocessor discharges these statically at each call site via flow-sensitive analysis; discharged calls are erased to zero runtime cost, undischarged calls fail the build.

**Linker gate via `atCompileTime`.** `proven.That` / `Returns` wrap their checks in a package-private `atCompileTime(func(){ ... })` helper. `atCompileTime` is declared via `//go:linkname` to an external symbol `_proven_atCompileTime` with no Go body. `gopls` and `go vet` see ordinary Go — IDE experience is always green. But `go build` / `go test` of any main or test target refuses to link without the preprocessor (which supplies the missing symbol during the toolexec pass). This is deliberate: forgetting the preprocessor is a loud link failure, never a silent loss of static checking.

**Test stubs + wiring verification via `proventest`.** `pkg/proventest/` supplies the `_proven_atCompileTime` symbol so test binaries link without the preprocessor. By default the stub is a no-op (matches production runtime: nothing executes). `proventest.WithChecks(fn)` flips a flag for the duration of `fn`, causing each `atCompileTime` block to execute its closure and panic (with a `proven.Violation` naming the failing predicate) if a predicate returns false. `proventest.AssertFails(t, pred, fn)` is the high-level helper: it runs `fn` under `WithChecks` and asserts that `pred` is the predicate that fires. `AssertAnyFailure(t, fn)` accepts any violation. These let tests verify "this parameter really is constrained by this predicate" — catching silent drift of compile-time-only requirements.

**Inference rules via `infer`.** `pkg/infer/` provides a fluent builder for declaring predicate implications the preprocessor will use to discharge obligations by subsumption:

```go
var _ = infer.From(isSmallPositive).To(isPositive)
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
```

Rules are trusted — no symbolic verification. A future `infertest.Verify` would property-test them.

## Authoritative docs

- [`docs/design.md`](docs/design.md) — the current authoritative design. Read this first.
- [`docs/companion-packages.md`](docs/companion-packages.md) — the three-package vision: `proven` (compile-time contracts, implemented), `prove` (runtime boundary validation, future), `infer` (inference rules implemented; comptime `Const` future).
- [`docs/concept.md`](docs/concept.md) — original motivation. API names are partially out of date (see design.md / companion-packages.md) but preprocessor architecture and motivation still apply.

## Background reading (historical / superseded)

Earlier design iterations. Do not build on these; retained to explain why the current shape exists.

- [`docs/parameter-constraint-syntax.md`](docs/parameter-constraint-syntax.md) — the A+C decision (generic wrapper + struct embedding) that was later overturned.
- [`docs/subsumption.md`](docs/subsumption.md) — proof-expression subsumption algebra required by the `Refined[P, T]` design. The current design handles cross-predicate implication via `infer.From(...).To(...)` declarations, not an expression algebra.
- [`docs/ide-integration.md`](docs/ide-integration.md) — the IDE friction analysis that drove the pivot away from type-level proof representations.

## Current implementation state

- `pkg/proven/` — runtime stubs: `That`, `Returns`, `And`, `Or`, `Not`, plus the `atCompileTime` link-gated helper. `Violation` struct and `PredicateName` helper for diagnostics.
- `pkg/proventest/` — test-only linker stub, `WithChecks`, `AssertFails`, `AssertAnyFailure`.
- `pkg/infer/` — fluent inference-rule builder (`From(...).Given(...).To(...)`); `Rule` marker type.
- `example/basic/` — end-to-end usage sketch with wiring-verification tests (`TestWiring_*`).
- **Preprocessor: not started.** Will follow the `rewire` shape (toolexec entry, per-package AST scan, rewrite). Flow-sensitive analysis is the new component; see `docs/design.md` for the narrow preprocessor scope.

## Conventions

- Predicates are ordinary `func(T) bool`. No marker methods, no wrapper types, no struct embedding.
- Multiple predicates in a `That` / `Returns` call are AND-composed (variadic). For OR or first-class predicate values, use `And` / `Or` / `Not`.
- Runtime behavior of `That` / `Returns` is only observable via `proventest.WithChecks` in test code. Production runs never reach the block body: either the preprocessor erased it, or the link failed.
- The preprocessor's job is narrow: scan bodies for `That` / `Returns`, build per-function obligation summaries, discharge them at call sites via flow analysis using `infer` rules as implication axioms, erase on success, fail on unproven. No type-level algebra, no SMT.
