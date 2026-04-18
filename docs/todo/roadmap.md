# Preprocessor roadmap

This is the resume-point for preprocessor work: what's done, what's next, and what each chunk looks like in broad strokes. Update as you go — if something shifts mid-implementation, update the roadmap rather than letting the doc rot.

## Where we are

**Done.**
- Package scaffolding: `pkg/proven`, `pkg/proventest`, `pkg/prove`, `pkg/infer`.
- Runtime APIs: `That`, `Returns`, `And` / `Or` / `Not`, `Violation`, `PredicateName`, `WithChecks`, `AssertFails`, `AssertAnyFailure`, `prove.That`, `prove.Must`, `infer.From(...).Given(...).To(...)`.
- Linker gate: `atCompileTime` declared via `//go:linkname` to the unresolved `_proven_atCompileTime` symbol.
- `cmd/proven/`: toolexec shim; all logic in `internal/preprocessor/`.
- `internal/preprocessor/`: AST-based preprocessor (`run.go` / `compile.go` / `provenstub.go`) + unit tests + golden-file e2e harness (`e2e_test.go`) running the real Go toolchain on fixtures under `testdata/cases/`, with an isolated `GOCACHE` per test run.
- Fixtures: `noop_ok`, `basic_proven_use_links_ok`.
- **Phase 1 (stub injection).** Any program using `proven.That` links under `-toolexec=proven`. The preprocessor detects `compile` invocations for `github.com/GiGurra/proven/pkg/proven`, parses the source files the compiler received, finds the `//go:linkname` declaration for `_proven_atCompileTime`, derives the stub's signature from the matched `FuncType`, writes a companion `.go` file to `$TMPDIR`, and appends it to the compile argv.
- **Phase 2 (per-package obligation scan).** `internal/preprocessor/scanner.go` builds an in-memory `PackageSummary`: for every `FuncDecl` whose body contains a `proven.That` or `proven.Returns` call on a direct parameter reference, record the parameter position → predicate list mapping (and a bag of return-postcondition predicates). Predicates are normalized as `Predicate{Pkg, Name}`, same-package references keyed by the scanned import path. Handles aliased proven imports, receiver-qualified method names, and silently skips unresolvable predicate expressions (inline combinators, function literals, arbitrary expressions) per the v1 scope in `docs/design.md`. Not yet wired into the compile path — Phase 3 consumes it at call sites.
- **Phase 3 (flow-sensitive discharge).** `internal/preprocessor/analyzer.go` walks each caller `FuncDecl` under a mutable `FactSet`, producing one `CallDischarge` per call to a function whose key is in the package summary. Each discharge lists `ParamDischarge{Required, Missing}` so Phase 5 can erase when Missing is empty and diagnose otherwise. All five fact sources from the roadmap are supported: preceding `if pred(x)` narrowing, early-return / panic guard with `if !pred(x) { escape }`, conjoined `&&` guards, postconditions from assignments of `proven.Returns`-annotated callees, and successful `prove.That` / `prove.Must` calls. Branch merge logic recognizes a branch that always escapes via return or panic and keeps the surviving branch's facts. Scope is deliberately narrow — direct-identifier subjects only, same-package free-function callees only, no full dataflow merge across complex control flow. Not yet wired into the compile path; Phase 5 will do that.
- **Phase 4 (inference-rule consumption).** Scanner harvests `var _ = infer.From(premise).[Given(context).]To(conclusion)` declarations into `PackageSummary.Rules` as `InferRule{From, Given *Predicate, To}` entries. Analyzer's discharge check falls back from direct fact lookup to backward-chaining through the rules: a required predicate on a variable is discharged if some rule concludes it, its From premise discharges (recursively), and its Given context (when present) also discharges on the same variable. Cycle-safe via a per-query visited set, so pathological mutual rules return false without recursing forever. Multi-hop chains and chains-through-Given both work.
- **Phase 5a (wiring + diagnostics).** The scanner+analyzer are now wired into the compile path. `planCompile` returns a `Plan{NewArgs, Cleanup, Diags}` and `planUserPackage` scans+analyzes every package the toolchain compiles; when any call-site obligation is undischarged the preprocessor emits one Go-standard `file:line:col: proven: undischarged predicate ...` diagnostic per missing predicate per param and exits non-zero before the real compile runs. `AnalyzePackage` integrates the scan+analyze in one pass, parsing each source once. prove.That's err-check pairing is now correct: the fact set only adds the postcondition on the err==nil side of a matching `if err != nil {escape}` or `if err == nil {body}` guard; an unpaired prove.That (or blank-identifier err) no longer falsely discharges. e2e fixtures under `testdata/cases/`: `preceding_check_ok`, `early_return_guard_ok`, `returns_flow_ok`, `prove_then_proven_ok` (build), `unguarded_fails`, `prove_then_error_not_discharged_fails` (build fails with diagnostic substring match).
- **Phase 5b (zero-cost rewriting).** `internal/preprocessor/rewriter.go` erases every discharged `proven.That` / `proven.Returns` call in source before the compiler sees it. Rewrite is byte-level, driven by the AST: `proven.That(...)` ExprStmts have their whole span blanked; `proven.Returns(v, preds...)` calls have the wrapper blanked with `v`'s bytes left in place. Edits are length-preserving, and nested `proven.Returns` inside `proven.Returns` collapses via innermost-first application. Every non-newline byte outside the preserved inner-argument span becomes a space, so cmd/compile's `file:line:col:` messages still point at the user's original columns — only a single sentinel line (`var _ = <provenAlias>.PredicateName`) is appended after the last original byte to keep the proven import in use when every call was erased. Rewritten files are written to per-call tempdirs and substituted for the originals in `planUserPackage`'s `Plan.NewArgs`; `Cleanup` reclaims the temp trees after the forwarded compile returns. Files without `proven.That` / `proven.Returns` targets keep their original paths — the rewriter touches only what it changes.
- **Phase 6 (cross-package obligations).** `internal/preprocessor/sidecar.go` persists each clean-analyzed package's `PackageSummary` as JSON at `<dir-of -o>/_pkg_.proven.json`. During a downstream package's compile, `planUserPackage` parses the compile's `-importcfg`, resolves each `packagefile <importpath>=<aPath>` to its sibling sidecar, and builds a `map[importPath]*PackageSummary` that the analyzer consults for cross-package callees and rules. `AnalyzeFunc` gained an `imports` parameter (nil for same-package-only contexts); callee resolution now handles `pkg.Foo` selectors via the file's import-alias map, `lookupCalleeSummary` dispatches between the current package and imports, and `CallDischarge` grew a `CalleePkg` field so diagnostics can render cross-package callees as `pkg.Foo`. Scope: single-build (the sidecar lives in Go's per-build tempdir — Phase X replaces this with GOCACHE-adjacent or content-addressed storage so summaries survive across builds and cache hits). e2e fixtures: `cross_package_ok` (main guards the call to `callee.Target` with `if callee.IsPositive(x)` → builds) and `cross_package_unproven_fails` (no guard → build fails with `undischarged predicate callee.IsPositive on parameter 0 of callee.Target`). The e2e harness's `copyGoTree` now walks fixture subdirectories so multi-package fixtures work.

**Not done.** Everything below is open work.

## Phases

Roughly sequential — each phase relies on the previous — but the harness tolerates incremental progress: add a fixture for a behavior, go red, implement until it's green.

### Phase 7 — `pkg/trust` local fact injection

**Goal.** Establish a compile-time proven fact on a value without any runtime verification. The programmer takes responsibility: used when the value has been validated through some other mechanism the preprocessor cannot see (external schema validator, prior business logic, type-state invariant), and a redundant runtime check would add cost for no safety gain.

A new sibling package `pkg/trust`, parallel to `pkg/prove`. The call-site naming symmetry makes the mechanism obvious at the point of use:

- `prove.That(v, preds...) (T, error)` — runtime check, error return.
- `prove.Must(v, preds...) T` — runtime check, panic on fail.
- `trust.That(v, preds...) T` — **no runtime check**; asserts the facts and propagates them downstream.

Shape:

- New `pkg/trust/trust.go` exposing `func That[T any](v T, preds ...func(T) bool) T`. The Go body just returns `v` — no `atCompileTime` wrapper, no predicate iteration. A program that uses only `trust.That` (no `proven.That` / `proven.Returns`) links without the preprocessor and produces a no-op pass-through; mis-uses are a programmer responsibility rather than a build-time failure.
- Analyzer treats `x := trust.That(raw, preds...)` as a fact injection: each listed predicate becomes a `Fact{Pred, Var: x}` in the caller's flow state, exactly parallel to how `prove.Must` is handled today but without the runtime side-effect. Inline use (`target(trust.That(raw, pred))` without an intervening assignment) is v2 scope, matching `proven.Returns`' and `prove.Must`'s current limitation.
- Rewriter erases `trust.That(v, preds...)` calls the same way it erases `proven.Returns(v, preds...)`: blank the wrapper, keep `v`'s bytes at their original column.

Distinction from `proven.Returns`:

- `Returns` lives inside a return statement and advertises a function-level postcondition via the package's `FuncSummary`, visible to every caller across packages via the Phase 6 sidecar.
- `trust.That` is local — it injects facts into the enclosing function's flow state, invisible to other functions or packages. If you want every caller to see the fact, use `Returns`.

**Fixtures.** `trust_local_discharge_ok` (trust.That establishes a predicate on a local variable, downstream target discharges, build succeeds), `trust_mismatched_predicate_fails` (trust.That asserts predicate P, target requires a different predicate Q, build fails with undischarged diagnostic).

### Phase 7.5 — Relational predicates (between-values)

**Goal.** Express and discharge obligations that relate *two or more* values rather than constrain one. The v1 predicate shape `func(T) bool` is strictly unary; domains like authorization, resource lifetimes, and protocol state frequently need "this operation on Executor `e` is only permitted when Controller `c` has authorized it" — a fact over the pair `(c, e)`, not over either alone.

Shape to work out (intentionally open — the sharp decisions emerge when we build a real example):

- **Predicate type.** A relational predicate would look like `func(T1, T2) bool` (or variadic). The scanner and analyzer both currently assume one subject per `Fact` (`Fact{Pred, Var}`). Extension: `Fact` becomes `Fact{Pred, Vars []string}`, indexed by subject position; direct equality still works on `(Pred, Vars)` tuples.
- **Obligation shape.** `proven.That` today wraps a single parameter. For relations we need something like `proven.Relation(pred, a, b)` or multi-argument `proven.That((a, b), pred)` — syntax TBD. The principle: declare which parameters (or returned values) participate, and which relational predicate constrains them.
- **Fact establishment.** A relational fact appears when a guard proves the relation on specific identifiers (`if permitsAccess(c, e) { ... }`), when a relational inference rule fires, or when a `trust.That`-style injection asserts it. The analyzer's guard walker must learn to capture multi-argument predicate calls.
- **Inference.** `infer.From(...).To(...)` today relates two unary predicates. Relational rules could be `infer.Relation(fromRel).To(toRel)` or reuse `From`/`To` with a relational `Rule` constraint type. Backward-chaining still works; the visited set just keys on `(Predicate, Vars)` pairs.

Example the phase should close:

```go
func Authorize(c Controller, e Executor) {
    proven.Relation(permitsAccess, c, e)
}

func Execute(c Controller, e Executor) {
    proven.Relation(permitsAccess, c, e)
    e.Run()
}

func handler(c Controller, e Executor) {
    if permitsAccess(c, e) {
        Authorize(c, e) // discharged
        Execute(c, e)   // discharged
    }
}
```

**Open design decisions before implementing.**

- Variadic arity or fixed-2? Fixed-2 covers the motivating use cases; variadic is more general but expands the analyzer's state space.
- How to name the surface API without colliding with existing `proven.That`. Options: `proven.Relation`, overloaded `proven.That((a, b), p)` (requires tuple-pack syntax), or a separate package `pkg/relate`.
- Whether relational predicates can be mixed with unary ones in the same `infer` rule (likely yes; the analyzer needs to know how many subjects each rule side carries).

**Fixtures.** `relation_guard_discharges_ok`, `relation_inference_ok`, `relation_unguarded_fails`, `relation_wrong_pair_fails`.

**Start after.** Phase 7 lands. Relational predicates are purely additive — they don't change Phases 1–6 — but they touch every scanner/analyzer invariant that currently assumes unary predicates, so doing them under a solid single-subject baseline keeps the change surface clean.

### Phase 8 — `infertest.Verify`

**Goal.** Property-test a declared inference rule on sample inputs to catch declared-but-false rules.

- New `pkg/infertest` (or fold into `proventest`).
- `Verify[T any](t *testing.T, rule infer.Rule, samples ...T)` asserts the implication holds on every sample.

**Fixtures.** Go unit tests, not e2e fixtures.

### Phase 9 — `infer.Const` (compile-time evaluation)

**Goal.** Zig-style comptime for pure expressions.

- Add `func Const[T any](x T) T` in `pkg/infer`.
- Execution-model decision (must pick): interpret a restricted subset in-process, spawn a generated helper binary that evaluates the expression and emits a literal, or wait for Go to ship comptime. Each has real tradeoffs.
- Verify purity; fail on I/O or unknown external calls.

**Fixtures.** `const_literal_ok`, `const_pure_function_ok`, `const_impure_fails`.

### Phase X — Shared compile-time relations / cacheable lookup (deferred)

**Goal.** Consolidate the stable, program-wide knowledge the preprocessor consumes into a single cacheable lookup so repeated per-compile work collapses to a table read, while keeping flow-sensitive per-caller analysis naturally parallel.

The right abstraction is intentionally not pinned by this phase — when we dig into it with Phases 6, 7, and 9 landed the concrete shape will be more obvious than anything we could commit to now. What this phase captures is the opportunity, the axis of separation, and some directions worth evaluating.

Context. `go build -p N` runs up to N package compiles concurrently, each invoking the preprocessor as a separate OS process. Today every invocation re-derives the same facts about shared packages independently: obligation signatures, declared inference rules, the implication closure the rules compose into, and (once Phase 6 lands) per-callee summaries for everything an imported package exports. As the module grows and Phases 6/7/9 layer on more per-package analyses, the duplicated work compounds.

Axis of separation the phase should preserve.

- **Stable / globally-shareable knowledge** — read-mostly, canonical, cacheable across compiles and across builds. The obligation summary for a function, the predicates declared in an inference rule, the implication graph those rules compose into, predicate-to-predicate compatibility (which predicates' outputs feed which obligations' inputs, transitively). These are properties of the source, not of any one caller.
- **Flow-sensitive caller analysis** — per-function, per-call-site. What facts does *this* caller establish before *this* call? Trivially parallel across packages once the stable-knowledge layer is a lookup, and not worth sharing — by the time you've looked up the callee's obligation signature, you already have what you need to answer the question locally.

What to resist. Layering caches on top of the existing scanner and rule consumer as they are today will look like it works and underdeliver. The real move is finding the clean unified representation of "what is known about this predicate / function / type", computing it once at the right scope (build? module? source hash?), and letting every subsequent query — from any preprocessor process, possibly across builds — reduce to a lookup.

Directions worth evaluating when the time comes.

- One central relation table: function → obligation signature; predicate → implied predicates (with context); predicate pair → compatibility. Stored once per build, read by every compile.
- Content-addressed cache keyed by source hash, invalidated the same way Go's own build cache invalidates.
- Rewire-style first-pass: a single-threaded scan of the whole module writes the table; every compile afterwards reads it. PID-keyed for the hot path, fallback to on-demand scan for any gap.
- Side-channel daemon that arbitrates a first-read-wins cache during a single `go build` invocation.

Boundaries to draw before implementing.

- Exactly which facts are stable vs flow-sensitive — the split above is directional, not precise.
- Who triggers the first pass: the preprocessor itself on first-invocation-per-build, or a separate explicit step the user runs, or lazily on-demand with coordination.
- Cache-invalidation strategy — rewire's "warn and require `go clean -cache`" vs an automatic content-hash scheme.

**Start after.** Phases 6 (cross-package obligations) and 9 (`infer.Const`) land. Those two make the performance shape concrete. Until then the best cache is the one we don't yet need.

## Known risks / open questions

- **Toolexec interface stability.** Go's `cmd/compile` args shape is not a stable API. Watch release notes; pin fallback behavior for unknown flags.
- **Diagnostic format.** Must match Go's `file:line:col: message` exactly for editor click-through to work.
- **Performance.** Per-compile AST parsing is cheap per package but multiplies across a large module. Benchmark after Phase 3; cache per-file scans by content hash if needed.
- **Cross-package summary location.** Must not interfere with Go's build cache. Decide this before Phase 6 starts — rewire's approach is a reasonable reference point.
- **`infer.Const` execution model.** The most open of the open questions. Defer until Phases 1-5 are solid.

## How to resume

1. Read `docs/design.md` for design; then this file for where you are.
2. `go test ./...` should be green with seed fixtures.
3. Pick the next phase. Add fixtures first (red), implement until green.
4. Update this file as phases complete — move the heading from a `### Phase N` under "Phases" to a "## Done" section, or inline mark it `(done)`.
