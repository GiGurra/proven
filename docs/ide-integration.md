# IDE integration

> **Status: largely superseded.** This document analyzed the IDE tension
> created by the `Refined[P, T]` design. The pivot to interface-based
> signatures (see [`design.md`](design.md)) eliminates the tension:
> `gopls` handles structural subsumption natively, no companion LSP or
> `Weaken` casts are required, and call sites look like plain Go. This
> file is kept as reference for the fallback path.

---


The preprocessor proves and rewrites at the `-toolexec` boundary. `gopls` — which runs Go's standard type checker and supplies diagnostics to every major editor — has no such boundary. It sees the raw source. Any call site where the user's proof requires subsumption to match the parameter's type (see `docs/subsumption.md`) appears to `gopls` as a type mismatch: the file goes red, autocomplete breaks, and the user cannot work productively.

This is a fundamental tension, not a bug. Every refinement-type system has confronted it. Below are the options we considered, ordered from least to most invasive, with honest assessments.

## Option 1 — Explicit `Weaken` casts

Provide `proven.Weaken[Q](x)`, an identity function at runtime that `gopls` sees as returning `Refined[Q, T]`. The preprocessor verifies the cast is sound (Q entails the target) and otherwise accepts it as a no-op.

```go
// Caller has Refined[And[A, B, C], int]; parameter wants Refined[And[A, B], int].
f(proven.Weaken[proven.And[A, B]](x))
```

**Pros.** IDE-friendly on day one; no tooling beyond the preprocessor. Makes subsumption visible in source — arguably a readability win.

**Cons.** Ceremony at every subsumption point. Users unfamiliar with the style may reach for `Weaken` defensively, cluttering code.

## Option 2 — Canonical-order convention

Enforce alphabetical ordering of combinator arguments (`And[A, B]` never `And[B, A]`) via an auto-formatter or lint rule. Solves commutativity but not set inclusion.

**Pros.** Cheap. Handles the single most common subsumption case at zero user cost.

**Cons.** Doesn't help when the caller has strictly more proofs than the parameter needs. `goimports`-style mechanical normalization only.

## Option 3 — Structural interface signatures (Approach B revisited)

Redesign parameter types to use interfaces with embedded marker interfaces:

```go
func SetPercent(p interface {
    proven.Positive
    proven.LessThan100
    Unwrap() int
}) {}
```

Go's structural typing gives subsumption for free — a value implementing more markers still satisfies the interface. `gopls` understands this natively with no extra work.

**Pros.** Zero IDE tooling. Subsumption is a Go-language feature we ride on, not a thing we implement.

**Cons.** Runtime cost (boxing, indirection, escape-to-heap). Combinatorial wrapper-type generation — one per unique proof-set × underlying type seen in the module. This is Approach B from `docs/parameter-constraint-syntax.md`, which we rejected on runtime grounds. Revisiting for IDE ergonomics is possible but should be driven by profiling evidence, not speculation.

## Option 4 — Shadow-source trick

Run `gopls` against a preprocessed view of the module. An integration binary rewrites sources on the fly — turning subsumption-requiring calls into explicit `Weaken` — and hands the rewritten tree to `gopls`.

**Pros.** `gopls` sees type-correct code while the user sees their original source.

**Cons.** Extremely fragile. File-position mapping between original and rewritten source must be byte-exact for go-to-definition, diagnostics, and inlay hints to work. Any source edit invalidates the shadow. Breaks on refactors. Not seriously considered.

## Option 5 — Companion LSP

A separate language server running alongside `gopls`. It does not re-implement Go. Instead:

- It suppresses specific `gopls` diagnostics that the preprocessor will discharge (subsumption-induced type mismatches where the subsumption is sound).
- It *adds* proven-specific diagnostics that `gopls` cannot see: unprovable obligations, failed subsumption, purity violations in `Const`.

VS Code and Neovim both support multiple LSPs per language. Users install the proven LSP alongside `gopls`; diagnostics merge at the editor level.

**Pros.** No `gopls` fork. Users get correct diagnostics. Additive — we do not regress IDE behavior for code that doesn't use proven.

**Cons.** We must re-implement the preprocessor's analysis in an incremental, latency-bounded LSP form. Must interoperate cleanly with `gopls` diagnostic IDs to suppress the right ones. This is a real project, though a tractable one once the preprocessor's core analysis exists.

## Option 6 — Fork or plug `gopls`

Carry a patch or plugin for `gopls` that teaches it subsumption rules. Realistically a fork, since `gopls` has no plugin architecture.

**Pros.** Best imaginable IDE experience.

**Cons.** Enormous ongoing maintenance. Users would have to install our `gopls`, not theirs — a nonstarter for adoption.

## Decision for v1

- **Option 1 (`Weaken`)** is the default. Honest, IDE-friendly, implementable as a single generic identity function alongside `Attest`/`TrustMe`.
- **Option 2 (canonical ordering)** ships as an auto-formatter/lint rule. Removes the most common reason users would need `Weaken` in the first place.
- **Option 5 (companion LSP)** is the medium-term target. If the project gets traction, this is where IDE polish lives.
- **Option 3 (structural interfaces)** — revisit only if profiling shows (A)'s ergonomics are blocking adoption in a way that a structural form would uniquely fix. Do not rebuild the API on speculation.
- **Options 4 and 6** are not planned. Too fragile / too expensive.

## What users should expect on day one

- Most call sites look fine in the IDE — when proof types match exactly (the common case), there is no subsumption to resolve.
- When the compiler would accept subsumption but the IDE does not, the user writes `proven.Weaken[Q](x)` to make the cast visible. This is ugly but explicit.
- When the IDE flags a call the preprocessor would *also* reject, the user sees the error immediately. (This is the useful, non-spurious case.)
- The `proven` binary includes a `check` subcommand that runs the full preprocessor analysis on demand. Use it when the IDE seems out of sync with the real build outcome.
- Pipelines, pre-commit hooks, and CI run the real preprocessor; the IDE is a best-effort mirror.

## Appendix — why not just rely on Go's type inference

Go's generic type inference could, in principle, infer that a call to `f(x)` where `x : Refined[And[A,B,C], T]` and `f` expects `Refined[And[A,B], T]` could be satisfied by implicit conversion. It cannot: Go does not synthesize implicit conversions for named struct types with different type arguments, and the language team has been firm that it will not. There is no "make it work by adjusting the Go spec" path; the preprocessor and/or explicit casts are the only levers.
