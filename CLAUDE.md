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
- [`docs/companion-packages.md`](docs/companion-packages.md) — the three-package vision: `proven` (compile-time contracts), `prove` (runtime boundary validation, `That`/`Must` implemented), `infer` (inference rules implemented; comptime `Const` future).
- [`docs/todo/roadmap.md`](docs/todo/roadmap.md) — **the resume-point for preprocessor work.** Where we are, which phases remain, what each phase looks like, open risks, and a "how to resume" checklist. Read before picking up preprocessor implementation.
- [`docs/go-language-findings.md`](docs/go-language-findings.md) — empirical Go-1.26 facts the design depends on (generic inference limits, type-parameter restrictions on alias/named-type RHS, `//go:linkname`-to-unresolved-symbol behavior). Revisit if Go changes.
- [`docs/concept.md`](docs/concept.md) — original motivation. API names are partially out of date (see design.md / companion-packages.md) but preprocessor architecture and motivation still apply.

## Background reading (historical / superseded)

Earlier design iterations. Do not build on these; retained to explain why the current shape exists.

- [`docs/parameter-constraint-syntax.md`](docs/parameter-constraint-syntax.md) — the A+C decision (generic wrapper + struct embedding) that was later overturned.
- [`docs/subsumption.md`](docs/subsumption.md) — proof-expression subsumption algebra required by the `Refined[P, T]` design. The current design handles cross-predicate implication via `infer.From(...).To(...)` declarations, not an expression algebra.
- [`docs/ide-integration.md`](docs/ide-integration.md) — the IDE friction analysis that drove the pivot away from type-level proof representations.

## Current implementation state

- `pkg/proven/` — runtime stubs: `That`, `Returns`, `And`, `Or`, `Not`, plus the `atCompileTime` link-gated helper. `Violation` struct and `PredicateName` helper for diagnostics.
- `pkg/proventest/` — test-only linker stub, `WithChecks`, `AssertFails`, `AssertAnyFailure`.
- `pkg/prove/` — runtime boundary validators: `That(v, preds...) (T, error)` and `Must(v, preds...) T`.
- `pkg/infer/` — fluent inference-rule builder (`From(...).Given(...).To(...)`); `Rule` marker type.
- `example/basic/` — end-to-end usage sketch with wiring-verification tests (`TestWiring_*`).
- `example/boundary/` — `prove` → `proven` flow example with wiring tests.
- `cmd/proven/` — toolexec binary scaffold (currently a pure forwarder).
- `internal/preprocessor/e2e_test.go` — golden-file harness over `testdata/cases/`.
- **Preprocessor behavior: not started.** Roadmap in [`docs/todo/roadmap.md`](docs/todo/roadmap.md). Seed fixtures under `testdata/cases/` validate the harness in both success and failure directions.

## Conventions

- Predicates are ordinary `func(T) bool`. No marker methods, no wrapper types, no struct embedding.
- Multiple predicates in a `That` / `Returns` call are AND-composed (variadic). For OR or first-class predicate values, use `And` / `Or` / `Not`.
- Runtime behavior of `That` / `Returns` is only observable via `proventest.WithChecks` in test code. Production runs never reach the block body: either the preprocessor erased it, or the link failed.
- The preprocessor's job is narrow: scan bodies for `That` / `Returns`, build per-function obligation summaries, discharge them at call sites via flow analysis using `infer` rules as implication axioms, erase on success, fail on unproven. No type-level algebra, no SMT.

## Keep the docs fresh

**Standing directive.** After every significant change — a new or renamed API, a new package, a completed or reshuffled roadmap phase, a new convention, a design reconsideration, or deletion of code that held important context — update the persisted docs **in the same commit**. The goal: a cold-state reader (no conversation memory) can reconstruct the project state and pick up the next task without re-deriving decisions.

Files to scan after each change; update whichever are affected:

- `README.md` — if the user-facing surface, example, or "How it works" section changes.
- `CLAUDE.md` — this file. "Current implementation state" list, conventions, standing directives.
- `docs/design.md` — if the authoritative design shifts or gains new clauses.
- `docs/companion-packages.md` — the per-package status/intent table.
- `docs/todo/roadmap.md` — move completed phases to "Done"; refine upcoming phases as their shape becomes clearer.
- `docs/go-language-findings.md` — if an empirical Go-language fact we rely on changes, or we discover a new one.

If a commit deletes code or infrastructure that held context (e.g. experiment packages), **the context it captured must move into a persistent doc in the same commit**. "It lived in that deleted file" is not a valid resume-point.

## Naming is not frozen

API identifiers, package boundaries, and file layouts here are chosen to read well at their point of use, not to be permanent. If a better name emerges during implementation or review — clearer reading, less ambiguity, better match to semantics — rename it. Update every reference (code, tests, docs, README snippets, example sketches) in the same commit, and note the prior name and rationale in the commit message so the change is easy to trace.

Past renames included `FailsWith` → `AssertFails` and `All`/`Any` → `And`/`Or`; each caught a real issue with the previous name after a little living with it. Future renames are welcome under the same discipline.
