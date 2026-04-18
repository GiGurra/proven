# CLAUDE.md

## Project overview

`proven` is a compile-time contract system for Go, delivered as a `-toolexec` preprocessor. Functions declare preconditions and postconditions as ordinary runtime assertions inside the body (`proven.That`, `proven.Returns`). The preprocessor discharges these statically at each call site via flow-sensitive analysis; discharged calls are erased to zero runtime cost, undischarged calls fail the build.

**Linker gate via `atCompileTime`.** `proven.That` / `Returns` wrap their checks in a package-private `atCompileTime(func(){ ... })` helper. `atCompileTime` is declared via `//go:linkname` to an external symbol `_proven_atCompileTime` with no Go body. `gopls` and `go vet` see ordinary Go — IDE experience is always green. But `go build` / `go test` of any main or test target refuses to link without the preprocessor (which supplies the missing symbol during the toolexec pass). This is deliberate: forgetting the preprocessor is a loud link failure, never a silent loss of static checking.

**Test stub via `proventest`.** `pkg/proventest/` supplies the `_proven_atCompileTime` symbol for test binaries. Test files import `proventest` (it's always a test-time-only import — production code never pulls it in). In addition to satisfying the link, `proventest.WithChecks(fn)` flips a global flag that causes each `atCompileTime` block to execute its closure while `fn` runs, turning failing predicates into runtime panics. This lets tests verify that assertions are wired to the intended predicates — "Transfer(-5, ...) really does trip `isPositive`" — without needing the preprocessor. `example/basic/syntax_test.go` contains several `TestWiringVerification_*` tests using this pattern.

## Authoritative docs

- [`docs/design.md`](docs/design.md) — the current authoritative design. Read this first.
- [`docs/companion-packages.md`](docs/companion-packages.md) — the three-package vision: `proven` (compile-time contracts), `prove` (runtime boundary validation), `infer` (compile-time evaluation / comptime). Only `proven` is currently implemented.
- [`docs/concept.md`](docs/concept.md) — original motivation. API names in it are partially out of date (see design.md / companion-packages.md for the current shape) but the preprocessor architecture and motivation still apply.

## Background reading (historical / superseded)

Earlier designs. Do not build on these; kept to explain why the current shape is what it is.

- [`docs/parameter-constraint-syntax.md`](docs/parameter-constraint-syntax.md) — the A+C decision (generic wrapper + struct embedding) that was later overturned.
- [`docs/subsumption.md`](docs/subsumption.md) — proof-expression subsumption algebra required by the `Refined[P, T]` design. Not relevant to the current design; the assertion-based approach has no subsumption.
- [`docs/ide-integration.md`](docs/ide-integration.md) — the IDE friction analysis that drove the pivot away from type-level proof representations.

## Experiments to consult before reopening settled questions

- `internal/inferenceexperiment/` — regression probe showing Go 1.26 cannot infer phantom type parameters from call-site context. Why `Refined[P, T]`-style APIs were rejected.
- `internal/embeddingexperiment/` — regression probe for the struct-embedding (C) design. Viable but heavyweight.
- `internal/aliasexperiment/` — regression probe for generic type aliases. Go rejects a type parameter as the RHS of an alias or named type; only non-generic per-combination aliases (`type PositiveInt = int`) work.
- `internal/manualexperiment/` — user probe confirming the generic-alias rejection.

## Current implementation state

- `pkg/proven/` — runtime stubs: `That`, `Returns`, `All`, `Any`, `Not`.
- `example/basic/` — end-to-end usage sketch: plain-Go signatures with `proven.That` preconditions, `proven.Returns` postconditions, caller-side discharge via preceding guards.
- Preprocessor: not started. Will follow the `rewire` shape (toolexec entry, per-package AST scan, rewrite). Flow-sensitive analysis is the new component.

## Conventions

- Predicates are ordinary `func(T) bool`. No marker methods, no wrapper types, no struct embedding.
- Multiple predicates in a `That` / `Returns` call are AND-composed (variadic). For OR or first-class predicate values, use `All` / `Any` / `Not`.
- Runtime behavior of `That` / `Returns` without the preprocessor is a hard requirement, not a debug convenience — it's the safety net when the preprocessor is misconfigured or absent.
- The preprocessor's job is narrow: scan bodies for `That` / `Returns`, build per-function obligation summaries, discharge them at call sites via flow analysis, erase on success, fail on unproven. No type-level algebra.
