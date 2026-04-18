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

**Not done.** Everything below is open work.

### Phase 5b — Zero-cost rewriting (follow-on)

**Goal.** Erase discharged `proven.That` / `proven.Returns` calls so the compiled binary has no residual closure allocation or function-call overhead at those sites.

- Already-discharged call sites are no-ops at runtime today (the atCompileTime symbol is a no-op stub), but each still allocates a closure and makes a linkname-resolved call. Phase 5b removes the calls from the AST in the temp sources the preprocessor hands to the compiler.
- `proven.That(x, preds...)` used as a statement: drop the ExprStmt.
- `proven.Returns(v, preds...)` used as an expression: replace with `v`.
- Preserve line/column information via `//line` directives so unrelated compile errors still point at the original source.

## Phases

Roughly sequential — each phase relies on the previous — but the harness tolerates incremental progress: add a fixture for a behavior, go red, implement until it's green.

### Phase 6 — Cross-package obligations

**Goal.** When package A calls package B's annotated function, A's discharge sees B's obligation summary.

- Emit per-package obligation summaries to a stable location during B's compile.
- Read summaries of transitively-imported packages during A's compile.
- Storage decision (must pick): alongside `.a` in GOCACHE (rewire-style), side-channel directory, or embedded in object files. Leaning GOCACHE-adjacent.

**Fixtures.** `cross_package_ok`, `cross_package_unproven_fails`.

### Phase 7 — `proven.Trust` boundary guard

**Goal.** Opt-in runtime check that establishes a proven fact downstream, for boundaries where static proof is impossible but `prove.That`'s error-return shape isn't wanted.

- Add `func Trust[T any](v T, preds ...func(T) bool) T` in `pkg/proven`.
- Preprocessor emits a runtime check at `Trust` call sites (panic on violation).
- Propagate the Trust result as a discharged-fact in Phase 3.

**Fixtures.** `trust_boundary_ok`, `trust_violation_panics_at_runtime`.

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

### Phase X — Parallel-safe sharing / shared-pass optimization (deferred)

**Goal.** Avoid redundant parse and scan work across concurrent preprocessor invocations without breaking the toolexec model, which assumes each compile is an independent process.

Context. `go build -p N` runs up to N package compiles concurrently, each invoking the preprocessor as a separate OS process. Today every invocation re-parses its own sources from disk and re-scans them, even when neighbour invocations just did the same work for the same imported packages (for the cross-package summaries Phase 6 will need) or even the same file (cgo-generated helpers compiled once per build variant, for example). As the module grows and Phases 6/7/9 layer on more per-package analyses, the duplicated work compounds.

Shape of the solution. Rewire's answer is a first single-threaded pass that scans the whole module's test sources once and caches the result keyed by parent PID. We can do the same for proven's module-wide obligation and inference-rule tables — that cost is paid once even though N compiles read the cache — and keep the per-compile work as just the caller-side discharge for that package's own call sites. Other shapes worth considering: a side-channel file server (a short-lived daemon that arbitrates the first-read-wins cache), embedding the summary as a byte slice in the compiled object, or leaning on GOCACHE with a proven-owned subdirectory keyed by source hash.

Boundaries to draw before implementing.

- Which work is per-package-pure (can run fully in parallel with no coordination — e.g. the caller-side discharge for one package, given a stable callee summary) vs which needs a synchronized join (the module-wide summary table, the implication graph, the cross-package fact propagation).
- Whether the first-pass is triggered by the preprocessor's own code (auto-run on first invocation per build) or by a separate explicit step the user runs.
- Cache-invalidation strategy — rewire's "warn and require `go clean -cache`" vs an automatic content-hash scheme.

**Start after.** Phase 6 (cross-package obligations) and Phase 9 (`infer.Const`) land. Those two add the biggest repeated work and so make the performance shape concrete.

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
