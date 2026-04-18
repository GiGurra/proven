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

**Not done.** Everything below is open work.

## Phases

Roughly sequential — each phase relies on the previous — but the harness tolerates incremental progress: add a fixture for a behavior, go red, implement until it's green.

### Phase 2 — Per-package obligation scan

**Goal.** For every `.go` source file being compiled, discover which functions declare preconditions (`proven.That`) / postconditions (`proven.Returns`) and with which predicates.

- Parse source via `go/parser`.
- Walk AST for `proven.That(arg, preds...)` and `proven.Returns(arg, preds...)`.
- Build a per-function summary: `paramIndex -> []predicateIdent`.
- In-memory data structure for now; Phase 6 persists it across packages.

**Fixtures.** Internal scanner unit tests in `internal/preprocessor/scanner_test.go`. No user-visible fixture yet.

### Phase 3 — Flow-sensitive discharge (caller side)

**Goal.** At each call site of an annotated function, determine which obligations the caller's flow context discharges.

- Track per-program-point facts: "at this point, predicate P holds on expression E".
- Accept facts from:
  1. Preceding `if pred(x) { call(x) }` narrowing.
  2. Early-return guard `if !pred(x) { return }; call(x)`.
  3. Conjoined `&&` guards.
  4. `proven.Returns`-annotated return values.
  5. `prove.That` success (err == nil branch).
- Match obligations against the fact set by predicate-function identity.

**Fixtures.**
- `preceding_check_ok`, `unguarded_fails`, `early_return_guard_ok`, `returns_flow_ok`, `prove_then_proven_ok`, `prove_then_error_not_discharged_fails`.

### Phase 4 — Inference-rule consumption

**Goal.** `infer.From(p).To(q)` declarations extend discharge by implication.

- Scan package-scope `var _ = infer.From(...).[Given(...).]To(...)` calls.
- Build an implication graph `P ⇒ Q` (optionally under context `C`).
- During Phase 3 discharge, when a direct match fails, walk the graph for an implication chain.

**Fixtures.** `infer_rule_discharges_ok`, `infer_rule_with_given_ok`, `infer_rule_missing_still_fails`.

### Phase 5 — Rewriting & diagnostics

**Goal.** Turn discharge results into build outcomes.

- Rewrite discharged `proven.That` / `proven.Returns` calls to no-ops, preserving line numbers.
- Emit Go-standard `file:line:col: message` diagnostics for undischarged calls and exit non-zero.
- Pass the modified source to the real compile tool; preserve position information for unrelated diagnostics.

**Fixtures.** Add expected.txt content (diagnostic substring) for the previously-red `unguarded_fails` and similar fixtures.

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
