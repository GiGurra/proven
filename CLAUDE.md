# CLAUDE.md

## Project overview

`proven` is a compile-time constraint checker for Go, delivered as a `-toolexec` preprocessor. It lets users attach refinement-type-style constraints (`Positive`, `NonEmpty`, predicate-carrying wrapper types) to values, proves them statically where possible, and injects runtime guards at explicit boundaries where it can't.

**Authoritative current design:** [`docs/design.md`](docs/design.md) — interface-based proof signatures plus struct-embedded proof markers. Read this first.

Background reading (historical / superseded, kept for context):

- [`docs/concept.md`](docs/concept.md) — the original idea: a toolexec preprocessor that adds refinement-type-style constraints to Go. Motivation and preprocessor architecture still apply.
- [`docs/parameter-constraint-syntax.md`](docs/parameter-constraint-syntax.md) — the A+C decision that was subsequently overturned, with the analysis of alternatives that led there.
- [`docs/subsumption.md`](docs/subsumption.md) — proof-expression subsumption algebra for the `Refined[P, T]` path. Not used by the current design (Go's structural typing handles subsumption).
- [`docs/ide-integration.md`](docs/ide-integration.md) — analysis of the IDE friction created by `Refined[P, T]`, which drove the pivot to interface-based signatures.

**Experiments to consult before reopening settled questions:**

- `internal/inferenceexperiment/` — regression test showing Go 1.26 cannot infer phantom type parameters from call-site context. This is why `proven.In(x)`-style APIs were rejected.
- `internal/embeddingexperiment/` — regression test showing the (C) struct-embedding patterns and the four subset-validation approaches (explicit narrowing, helper method, generic constraint, interface value) all compile cleanly.

## Status

Pre-alpha. Repo currently contains only design docs and this file. No Go module, no binary, no checker.

The first slice to build (see concept.md "Minimum viable slice") is:

1. `pkg/proven/` — public API (`Refined[P, T]`, `Attest[P]`, `TrustMe[P]`, `Const`).
2. `cmd/proven/` — toolexec binary following the dispatcher pattern from `rewire` (detect toolexec mode vs CLI mode by first-arg shape).
3. `internal/scanner/` — per-package scan for `proven.*` references.
4. `internal/checker/` — trivial proof engine (literal vs. predicate) for the first slice.
5. `internal/rewriter/` — guard injection for `TrustMe`.
6. `example/` — one end-to-end module demonstrating `Positive`.

## Related repos

- **`~/git/rewire`** — the toolexec pipeline this project reuses. Its `internal/toolexec/`, `internal/rewriter/`, and `cmd/rewire/main.go` are the reference for scanning, rewriting, and the tool-mode dispatch. When adding anything toolexec-shaped here, check how `rewire` did it first.
- **`~/git/fl`** — the earlier thought experiment on refinement types and comptime. The design ideas motivated this project; its implementation did not (fl has no compiler).

## Conventions

- CLI uses `github.com/GiGurra/boa` (same as `rewire`).
- AST rewriting generates replacement code as text via `fmt.Sprintf` + `go/parser`, not manual AST node construction. (Same as `rewire`.)
- Test files (`_test.go`) are not rewritten unless they explicitly opt in — `proven` operates on production code.
- Keep the public API surface small. Refined types, attestation, trust-me, const. No convenience wrappers until the core works.

## Design principles

- **Fail the build, don't warn.** If the checker can't prove an obligation and the user didn't use `TrustMe`, it is a compile error.
- **Zero runtime cost on the proven path.** `Refined[P, T]` must compile to nothing when the proof succeeds. Any runtime overhead belongs at boundaries.
- **One import.** Users add `proven` to one place and turn it on via `GOFLAGS`. No `go:generate`, no committed generated files unless genuinely unavoidable.
- **Go-idiomatic errors.** When the checker rejects a build, the error message must point at the call site, name the obligation, and suggest the minimal fix (precondition, `Attest`, or `TrustMe`).

## Open design questions

Do not resolve these silently — surface them when the work touches them:

- SMT backend choice (Z3 vs. pure-Go solver for the common subset).
- Cross-package constraint propagation (sidecar summary files vs. same-package restriction).
- Generic constraints on type parameters (explicitly deferred past v1).
- Language-server integration for predicate doc comments (deferred past v1).
