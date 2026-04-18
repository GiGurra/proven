# CLAUDE.md

## Project overview

`proven` is a compile-time contract system for Go, delivered as a `-toolexec` preprocessor. Functions declare preconditions and postconditions as ordinary runtime assertions inside the body (`proven.That`, `proven.Returns`). The preprocessor discharges these statically at each call site via flow-sensitive analysis; discharged calls are erased to zero runtime cost, undischarged calls fail the build.

**Linker gate via `atCompileTime`.** `proven.That` / `Returns` wrap their checks in a package-private `atCompileTime(func(){ ... })` helper. `atCompileTime` is declared via `//go:linkname` to an external symbol `_proven_atCompileTime` with no Go body. `gopls` and `go vet` see ordinary Go — IDE experience is always green. But `go build` / `go test` of any main or test target refuses to link without the preprocessor (which supplies the missing symbol during the toolexec pass). This is deliberate: forgetting the preprocessor is a loud link failure, never a silent loss of static checking.

**Test stubs + wiring verification via `proventest`.** `pkg/proventest/` supplies the `_proven_atCompileTime` symbol so test binaries link without the preprocessor. By default the stub is a no-op (matches production runtime: nothing executes). `proventest.WithChecks(fn)` flips a flag for the duration of `fn`, causing each `atCompileTime` block to execute its closure and panic (with a `proven.Violation` naming the failing predicate) if a predicate returns false. `proventest.AssertFails(t, pred, fn)` is the high-level helper: it runs `fn` under `WithChecks` and asserts that `pred` is the predicate that fires. `AssertAnyFailure(t, fn)` accepts any violation. These let tests verify "this parameter really is constrained by this predicate" — catching silent drift of compile-time-only requirements.

**Inference rules via `infer`.** `pkg/infer/` provides a fluent builder for declaring predicate implications the preprocessor will use to discharge obligations by subsumption:

```go
var _ = infer.From(isSmallPositive).To(isPositive)
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

// Every slot is variadic and AND-composes, same as proven.That.
var _ = infer.From(isEven, isPositive).To(isNonNeg, isNonZero)
```

Rules are trusted — no symbolic verification. `infertest.Verify` / `VerifyApplies` (implemented) property-test them against sample inputs.

## Authoritative docs

- [`docs/design.md`](docs/design.md) — the current authoritative design. Read this first.
- [`docs/companion-packages.md`](docs/companion-packages.md) — the four-package vision: `proven` (compile-time contracts), `prove` (runtime boundary validation), `trust` (local fact injection), `infer` (inference rules). Compile-time evaluation of pure expressions was explored and declared out of scope — see [`docs/comptime.md`](docs/comptime.md).
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
- `pkg/trust/` — the "trust me" escape hatch: `That(v, preds...) T` injects local facts with no runtime check, `Returns(v, preds...) T` combines that with `proven.Returns`-style postcondition advertisement (no site verification). Programmer takes responsibility for correctness.
- `pkg/infer/` — fluent inference-rule builder (`From(...).Given(...).To(...)`); `Rule` carries `func(any) bool` wrappers over the typed predicates plus `Check`/`Applies` methods.
- `pkg/infertest/` — property-test sibling: `Verify[T]` and stricter `VerifyApplies[T]` over `infer.Rule` catch declared-but-false inference rules at test time.
- `example/basic/` — end-to-end usage sketch with wiring-verification tests (`TestWiring_*`).
- `example/boundary/` — `prove` → `proven` flow example with wiring tests.
- `cmd/proven/` — toolexec shim. All behavior lives in `internal/preprocessor`.
- `internal/preprocessor/` — AST-based preprocessor package (`run.go` / `compile.go` / `provenstub.go` / `scanner.go` / `analyzer.go` / `rewriter.go` / `sidecar.go` / `userpkg.go`) plus its `e2e_test.go` golden-file harness over `testdata/cases/`, `provenstub_test.go`, `scanner_test.go`, `analyzer_test.go`, `infer_test.go`, `rewriter_test.go`, and `sidecar_test.go`.
- **Preprocessor behavior: Phases 1–8 and 11a done.** Phase 1 injects a companion `.go` file into pkg/proven's compile providing the `_proven_atCompileTime` linker symbol as a no-op. Phase 2 (`ScanPackage`) builds a per-package `PackageSummary{Funcs, Rules}` of parameter preconditions, return postconditions, and inference-rule implications. Phase 3 (`AnalyzeFunc`) walks each caller under a mutable `FactSet`, producing `CallDischarge{CalleePkg, CalleeKey, Params []ParamDischarge{Required, Missing}}` per call; fact sources are preceding `if pred(x)`, early-return/panic guards, conjoined `&&`, `proven.Returns` postconditions, and successful `prove.That` / `prove.Must` — the latter two correctly gate on the err-check's branch. Phase 4 extends discharge with backward-chaining over `infer.From(p).[Given(c).]To(q)` rules, cycle-safe and chained. Phase 5a wires it all into the compile path: `planCompile` returns a `Plan{NewArgs, Cleanup, Diags}`; `planUserPackage` scans+analyzes every user-package compile and emits Go-standard `file:line:col: proven: undischarged predicate <p> on parameter N of <callee>` diagnostics before exiting non-zero. Phase 5b (`rewriter.go`) makes discharged call sites zero-cost by blanking every `proven.That` / `proven.Returns` call span in the temp source the compiler sees (length-preserving, so cmd/compile error columns still match user source; one appended sentinel keeps the proven import in use). Phase 6 (`sidecar.go`) handles cross-package obligations: each clean-analyzed package writes its `PackageSummary` as JSON next to its `_pkg_.a`; downstream compiles parse `-importcfg` to resolve each imported package's sidecar and consult it through an `imports map[string]*PackageSummary` passed to `AnalyzeFunc`. `pkg.Foo` selector calls are resolved via the file's import-alias map into `(CalleePkg, CalleeKey)`. Scope is single-build — cached .a artifacts reused by Go's build cache do not currently carry their sidecars forward; Phase X will address cross-build persistence. Phase 7 adds `pkg/trust` with `trust.That(v, preds...) T`: the analyzer treats each listed predicate as a `Fact{Pred, Var: LHS}` on assignment and the rewriter erases the call (same shape as `proven.Returns`). No runtime check, programmer owns correctness. Phase 7.5 (relations between values — tuple-first) is zero preprocessor code: relations over multiple subjects are expressed by packing the subjects into a domain struct and writing the predicate as unary over that struct; every existing capability applies unchanged. See [`docs/relations.md`](docs/relations.md) for the full exploration and the deferred currying / `proven.Relation` alternatives. Phase 8 introduces `pkg/infertest` with `Verify[T]` / `VerifyApplies[T]` — property-test helpers that run a declared `infer.Rule` against caller-supplied samples and report counter-examples (a sample where premise and Given both hold but conclusion fails). `infer.Rule` now carries `func(any) bool` wrappers over its typed predicates plus `Check`/`Applies` methods; the type stays non-generic so the existing `var _ infer.Rule = ...` surface is unchanged. Fixtures under `testdata/cases/` cover all five fact sources, same-package, cross-package, trust-based fails, and tuple-subject relations. Phase 11a adds inline `proven.And` and `proven.Or` at every obligation and fact site (`proven.That` / `proven.Returns` / `trust.Returns`, `prove.That` / `prove.Must`, `trust.That`). `proven.And` decomposes into leaf obligations on ingest so the analyzer, sidecar, and rewriter need zero changes. `proven.Or` is stored in a parallel `ParamOrs` / `ReturnOrs` slot on `FuncSummary`; the `FactSet` grows a disjunctive-fact map keyed by variable plus canonical alt-order-insensitive `pkg|name,…` keys. `dischargedOr` succeeds when either any alt leaf discharges or a structural-equal Or-fact was planted. `proven.Or` is still rejected inside `infer.From/Given/To` (would synthesize multiple rules) and `proven.Not` remains rejected everywhere (needs negation-fact work). Roadmap in [`docs/todo/roadmap.md`](docs/todo/roadmap.md).

## Conventions

- Predicates are ordinary `func(T) bool`. No marker methods, no wrapper types, no struct embedding.
- Multiple predicates in a `That` / `Returns` call are AND-composed (variadic). For OR or first-class predicate values, use `And` / `Or` / `Not`.
- Runtime behavior of `That` / `Returns` is only observable via `proventest.WithChecks` in test code. Production runs never reach the block body: either the preprocessor erased it, or the link failed.
- The preprocessor's job is narrow: scan bodies for `That` / `Returns`, build per-function obligation summaries, discharge them at call sites via flow analysis using `infer` rules as implication axioms, erase on success, fail on unproven. No type-level algebra, no SMT.
- **Preprocessor architecture: parse, don't template.** Every pass reads the source files the Go toolchain handed the compiler and works from their AST (`go/parser`, `go/ast`, `go/printer`) — shape follows `github.com/GiGurra/rewire`. Synthesized files are emitted to `$TMPDIR` and appended to the compile argv; the on-disk source tree is never modified. Do not hardcode symbol names, signatures, or textual templates that duplicate what is already in the source — derive them from the AST so API evolution in `pkg/proven` / `pkg/infer` flows through mechanically.

## Developing the preprocessor

**Cache discipline.** Go's build cache key does **not** include the toolexec binary's effect on source. A cached `pkg/proven.a` from a plain `go test` (no stub) and one from a `-toolexec=proven` build (with stub) have the same key — whichever ran first wins. Symptoms of the mismatch:

- `relocation target _proven_atCompileTime not defined` — a toolexec build reused a stub-less artifact.
- `duplicated definition of symbol _proven_atCompileTime, from ... proventest ... and ... proven` — a non-toolexec test binary (pulling in `proventest`) reused a stub-containing artifact.

Three protections are in place so you rarely have to think about this:

1. The e2e harness sets `GOCACHE` to a harness-owned tempdir, so `go test ./...` and `go test ./internal/preprocessor/...` do not cross-contaminate.
2. When running toolexec builds manually against an existing module, keep a dedicated `GOCACHE` for them: `GOCACHE=$(mktemp -d) go build -toolexec=/path/to/proven ./...` — or run `go clean -cache` between preprocessor-on and preprocessor-off flows.
3. Rebuild the preprocessor after any change to `cmd/proven` or `internal/preprocessor` before re-running toolexec builds: `go install ./cmd/proven` (or whatever wrapper you invoke). The e2e harness already does this in `TestMain`; manual loops must do it explicitly, because the old binary on `$PATH` will not auto-update.

If Phase 6 lands, rewire's `$GOCACHE/rewire/targets-*.hash` pattern (a sentinel file that the preprocessor checks and asks the user to `go clean -cache` on mismatch) is the reference point — see `/Users/johkjo/git/rewire/internal/toolexec/cacheinval.go`.

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
