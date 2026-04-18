# Preprocessor roadmap

This is the resume-point for preprocessor work: what's done, what's next, and what each chunk looks like in broad strokes. Update as you go — if something shifts mid-implementation, update the roadmap rather than letting the doc rot.

## Where we are

**Done.**
- Package scaffolding: `pkg/proven`, `pkg/proventest`, `pkg/prove`, `pkg/infer`.
- Runtime APIs: `That`, `Returns`, `And` / `Or` / `Not`, `Violation`, `PredicateName`, `WithChecks`, `AssertFails`, `AssertAnyFailure`, `prove.That`, `prove.Must`, `infer.From(...).Given(...).To(...)` (every slot variadic, AND-composed).
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
- **Phase 7 (`pkg/trust` local fact injection).** New sibling package `pkg/trust` exposes `func That[T any](v T, preds ...func(T) bool) T` — a no-op pass-through at runtime (body just returns `v`, no `atCompileTime` wrapper), so programs using only `trust.That` link without the preprocessor. Analyzer treats `x := trust.That(raw, preds...)` as a fact injection: each listed predicate becomes a `Fact{Pred, Var: x}` in the caller's flow state, parallel to `prove.Must` but without the runtime side-effect. Rewriter erases `trust.That(v, preds...)` calls the same way it erases `proven.Returns(v, preds...)`: blank the wrapper, keep `v`'s bytes at their original column. Distinction from `proven.Returns`: `Returns` advertises a function-level postcondition via the Phase 6 sidecar (visible to every caller across packages); `trust.That` is local — facts are invisible to other functions. Inline use without an intervening assignment is v2 scope, matching `proven.Returns`' and `prove.Must`'s current limitation. e2e fixtures: `trust_local_discharge_ok` (trust.That establishes a predicate on a local, downstream target discharges, build succeeds), `trust_mismatched_predicate_fails` (trust.That asserts P, target requires Q, build fails with undischarged diagnostic).
- **Phase 7.5 (relations between values — tuple-first).** Express obligations over multiple values by packing participating subjects into a domain struct and writing the predicate as unary over that struct. `func canModify(a AuthCtx) bool` replaces what would be `func canModify(s Session, u User, r Resource) bool`. Every existing capability — guards, early-return, `proven.Returns`, `prove.That`/`Must`, `trust.That`, `infer.From/Given/To`, the Phase 6 cross-package sidecar, Phase 5b erasure — applies to tuple subjects unchanged because the AST shapes are identical to unary-primitive subjects. Zero preprocessor code changes: Phase 7.5 ships as documentation (`docs/relations.md`) plus three e2e fixtures verifying the pattern end-to-end: `tuple_relation_guard_discharges_ok`, `tuple_relation_unguarded_fails`, `tuple_relation_trust_discharges_ok`. Two alternative approaches were explored and deferred — currying (`f(s, u)(r)`, reuses `proven.That` but needs `resolvePredicate` to handle `CallExpr` and analyzer to substitute captured args at call sites) and an explicit `proven.Relation(pred, subjects...)` API (cleaner multi-premise inference, but requires a new runtime API, a new `Fact{Vars []string}` shape, and reflection-based test-mode dispatch). See [`docs/relations.md`](../relations.md) for the full exploration, AST shapes for each approach, and the revisit triggers that would reopen the decision.
- **Phase 8 (`infertest.Verify`).** New sibling package `pkg/infertest` exposes `Verify[T any](t infertest.TestingT, rule infer.Rule, samples ...T)` and a stricter `VerifyApplies[T any]` variant that additionally fails when every sample misses the premise. `infer.Rule` gained private `func(any) bool` wrappers over its premise, Given, and conclusion predicates (lifted by a generic `wrapAny[T]`), plus a `Check(sample any) bool` method and an `Applies(sample any) bool` method — the rule type stayed non-generic so the package-scope `var _ infer.Rule = infer.From(...).To(...)` surface remained unchanged. Verify uses a minimal `TestingT` interface (just `Helper` + `Errorf`) so tests can substitute a recording fake without `*testing.T`'s nominal identity getting in the way. Catches declared-but-false rules early: the classic `infer.From(isEven).To(isPositive)` counter-example at `-4` is caught by the test that declares it. Unit tests in `pkg/infertest/infertest_test.go` cover sound rules, unsound rules, Given-conditioned rules (both valid and invalid), vacuously-true sample sets (silently accepted by `Verify`, rejected by `VerifyApplies`), and partially-hitting sample sets.

**Not done.** Everything below is open work.

## Phases

Roughly sequential — each phase relies on the previous — but the harness tolerates incremental progress: add a fixture for a behavior, go red, implement until it's green.

### Phase 9 — Performance, caching, multi-phase preprocessing

**Goal.** Make the preprocessor fast and correct on non-toy codebases. Two threads live under this phase: a **latent correctness bug** around Go's build cache reusing cached `.a` artifacts that do not carry their proven sidecars forward, and a **performance opportunity** where every compile re-derives the same stable facts (per-file AST parses, imported-package sidecar unmarshals, inference-rule closures) independently of every other compile in the same build. A third thread — a rewire-style first-invocation-per-build module scan that collapses most of the duplicated work to a table read — is worth evaluating once we have numbers.

Scope before design: **measure first**. Phase 9 begins with a profiling pass on a realistic example (see `docs/perf.md` if it exists, otherwise the design task creates it) so the subsequent implementation work is aimed at the hot path, not at plausible-sounding abstractions.

Concrete threads, roughly in order of priority:

- **Cache correctness (the latent bug).** Go's build cache key does **not** include the toolexec binary's effect, so a cached `pkg/foo.a` reused across builds does not regenerate its `_pkg_.proven.json` sidecar. Downstream packages that import `pkg/foo` under `-toolexec=proven` then see no sidecar and silently lose cross-package obligation checks. Design options include: requiring `go clean -cache` between toolexec-on and toolexec-off modes (rewire's approach — loud but manual), writing the sidecar into a location keyed by something the Go cache already invalidates, or a content-hash-based cache that the preprocessor itself manages.
- **Per-build duplicate work.** Within one `go build -p N`, each compile loads and unmarshals every imported package's sidecar independently. The same JSON file is parsed N times. A per-build shared cache (on-disk at a stable location, or daemon-mediated) collapses this to one parse and many reads.
- **Per-invocation duplicate work.** Every toolexec invocation re-parses every source file twice — once for the scanner, once for the rewriter and analyzer. A single parse shared across passes drops the invocation cost roughly in half.
- **Rewire-style first pass.** The largest win would be a first-invocation-per-build full-module scan that precomputes every package's summary and the implication-graph closure, then every subsequent compile reads from the precomputed index. Worth evaluating once we have per-package parse costs and rule-lookup costs measured.

Axis of separation the phase should preserve (from the original Phase X framing, still valid):

- **Stable / globally-shareable knowledge** — read-mostly, canonical, cacheable across compiles and across builds. Per-function obligation summaries, declared inference rules, the implication closure those rules compose into. These are properties of the source, not of any one caller.
- **Flow-sensitive caller analysis** — per-function, per-call-site. What facts does *this* caller establish before *this* call? Trivially parallel across packages once the stable-knowledge layer is a lookup, and not worth sharing — by the time you have looked up the callee's obligation signature, you already have what you need to answer the question locally.

Cache-invalidation strategy has to be decided: rewire's "warn and require `go clean -cache`" vs an automatic content-hash scheme the preprocessor manages. Rewire's pragma is honest about the Go cache's limits; an automatic scheme is nicer when it works but has its own failure modes.

**Start after.** Measurement. Profile first, design second, code third. The best cache is the one we know we need.

### Phase 10 — Methods as predicates

**Goal.** Accept bound methods as predicate arguments, expanding the expressive range of the contract system beyond bare functions.

Current state: the scanner's `resolvePredicate` recognizes `*ast.Ident` (same-package free function) and `*ast.SelectorExpr` with a package-alias receiver (`pkg.Fn`). It does NOT recognize a selector whose receiver is a value or type — e.g. `myReceiver.IsValid` or `(*MyType).IsValid`. These are legitimate `func(T) bool` values at the Go type level, and the analyzer has no principled reason to reject them. Strict mode currently rejects them with `proven: argument to proven.That must be a named function or pkg.Name selector` (from the change landed alongside the lambda-rejection fix).

Design work the phase has to cover:

- **Predicate identity.** A method expression `MyType.IsValid` has a stable package + receiver + method triple. A method value `instance.IsValid` has the same stable triple at the declaration site, but is bound at a specific instance; two instances call the same method. For cross-package discharge, identity must be "same method on same receiver type" — different bindings of the same method are the same predicate. This matches Go's semantics but extends `Predicate{Pkg, Name}` to `Predicate{Pkg, Recv, Name}`.
- **Receiver in the guard.** `if x.IsValid() { Target(x) }` — the analyzer's guard walker sees a CallExpr with a SelectorExpr Fun. It must resolve the selector to a method identity, decide whether the method is a predicate (signature `() bool` at call site, since the receiver is bound), and plant a fact.
- **Cross-package sidecar.** The `Predicate` shape in JSON needs the receiver field. Straightforward but breaks sidecar compatibility — a migration note or version bump on the sidecar format will be needed.
- **proventest.AssertFails / Verify.** These take `pred func(T) bool`. A method value `x.IsValid` already has that type in Go, so the test-time API needs no change. A method expression `MyType.IsValid` has type `func(MyType) bool` and also works.

Expected shape after the phase:

```go
type Session struct { id string }
func (s Session) IsAuthenticated() bool { return s.id != "" }

func modifyResource(s Session) {
    proven.That(s, Session.IsAuthenticated) // method expression — predicate identity is (pkg, Session, IsAuthenticated)
}

func handler(s Session) {
    if s.IsAuthenticated() {
        modifyResource(s) // discharged
    }
}
```

Out of scope for Phase 10: pointer vs value receiver equivalence (treating `Session.IsValid` and `(*Session).IsValid` as the same predicate — v2 if ever), generic methods (deferred with generic predicate support generally).

### Phase 11a — Inline combinators (done)

**Status:** shipped. Inline `proven.And` and `proven.Or` are accepted at obligation and fact sites (`proven.That` / `proven.Returns` / `trust.Returns`, `prove.That` / `prove.Must`, `trust.That`) and inline `proven.And` is additionally accepted inside every `infer.From/Given/To` slot (Or in an infer slot is still rejected — see below).

- **And.** `resolveAndFlat` walks combinator trees recursively and emits one leaf predicate per tree leaf; nested `And` flattens fully. `proven.That(x, proven.And(a, b))` is stored as the two-leaf list `[a, b]`, identical to `proven.That(x, a, b)`. The analyzer, sidecar, and rewriter needed zero changes — the `And` is gone before they see it.
- **Or.** An Or-obligation is stored in a parallel `ParamOrs` / `ReturnOrs` slot on the summary. The analyzer's `dischargedOr` succeeds when either (a) any alt leaf discharges through the normal leaf path, or (b) a structurally-matching Or-fact was planted (alt order-insensitive, via a canonical `pkg|name,…` key). Or-facts are planted by `prove.That` / `prove.Must` / `trust.That` / `trust.Returns` via `TrailingPreds`, and by the callee's `ReturnOrs` postcondition when the caller assigns the result. Or's arguments must be named leaves — nested combinators inside Or are v2 scope and fail the build with a dedicated diagnostic.
- **Infer slots.** Or inside `infer.From/Given/To` stays rejected: a disjunctive premise or conclusion synthesizes multiple rules, and silently splitting one rule into many would obscure the declared shape. Users are asked to declare one rule per disjunct.
- **Not.** Still rejected at every site — a negation-fact representation is v2 scope.

Fixtures under `testdata/cases/`: `predicate_inline_and_ok`, `predicate_inline_and_nested_ok`, `predicate_inline_and_missing_leaf_fails`, `infer_from_inline_and_ok`, `or_obligation_via_leaf_guard_ok`, `or_obligation_undischarged_fails`, `or_fact_from_prove_discharges_or_ok`, `or_fact_from_trust_discharges_or_ok`, `or_infer_slot_rejected_fails`, `not_inline_still_rejected_fails`.

Not done (explicitly): nested combinators inside Or (e.g. `proven.Or(proven.And(a, b), c)` — CNF/DNF normalization is v2). Or inside infer slots (could be split into rules but the current design prefers explicit declarations). `proven.Not` at every site. A user-function returning an anonymous predicate (`var p = makePred(x)` or inline `proven.That(v, makePred(x))`) — the scanner cannot resolve such a call to a stable leaf identity.

### Phase 11 — Closures / lambdas as predicates

**Goal.** Accept function literals as predicate arguments, without silently weakening the contract.

Current state: function literals are rejected at build time because they have no stable package + name identity, which means the preprocessor cannot correlate a lambda used as an obligation in one place with the same-or-compatible lambda used as a fact in another. Phase 11 asks: can we give lambdas a useful compile-time identity that enables same-file or same-package discharge?

Two sub-questions the design has to close (inline combinators are split out into Phase 11a above):

- **Identity.** A lambda has no name. Possible bases: the lambda's source text (after gofmt normalization), its AST hash, its syntactic position (`file:offset`), or a content-hash of its body. Each has tradeoffs: source-text and AST-hash give structural equality (two identical lambdas in different files are "the same"), position-based gives per-definition identity (different lambdas at different positions are distinct even when textually identical). Which matches user intent?
- **Scope of use.** Even with identity, a lambda defined in file A can't realistically be discharged by a fact established in file B unless both reference the same declared identifier — so the practical scope is "lambdas used multiple times within one file, or assigned to a package-scope var before use." The latter is already the recommended workaround today (`var p = func(x int) bool { ... }; proven.That(v, p)`), which means it already works — a named var binding a function literal passes the Ident resolver. The real gap is single-use inline `proven.That(v, func(x int) bool { ... })` which the preprocessor never sees at any other site.

Out of scope for Phase 11: closures that capture enclosing variables (their bodies reference non-parameter state; cross-call identity is essentially impossible), lambdas that change over time (redefined in different builds — their identity would shift, breaking cache coherence).

## Out of scope

- **`infer.Const` (compile-time evaluation of pure expressions).** Was briefly on the roadmap as Phase 9 under the argument that `infer` is where build-time deduction lives. When we sat down to design it we concluded it does not belong in this project: the contract system's value proposition (predicate obligations and their discharge) has essentially nothing in common with Zig-style comptime (running pure code at build time to emit literals), and bundling them under one package obscures both. The full exploration — 12 candidate execution models, real code spikes on four of them, final ranking — lives in [`docs/comptime.md`](../comptime.md), preserved for anyone who wants to pick this up as its own project.

## Known risks / open questions

- **Toolexec interface stability.** Go's `cmd/compile` args shape is not a stable API. Watch release notes; pin fallback behavior for unknown flags.
- **Diagnostic format.** Must match Go's `file:line:col: message` exactly for editor click-through to work.
- **Cross-package summary location vs Go's build cache.** The original framing assumed a sidecar adjacent to `_pkg_.a` was sufficient. It is not, because Go's cache reuses the `.a` without re-running toolexec. Phase 9 owns this.

## How to resume

1. Read `docs/design.md` for design; then this file for where you are.
2. `go test ./...` should be green with seed fixtures.
3. Pick the next phase. Add fixtures first (red), implement until green.
4. Update this file as phases complete — move the heading from a `### Phase N` under "Phases" to a "## Done" section, or inline mark it `(done)`.
