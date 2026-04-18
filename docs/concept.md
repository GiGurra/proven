# proven: concept

> **Note.** This document captures the original motivation and the preprocessor architecture. Some of the specific API names below (`proven.Refined[P, T]`, `proven.Const`, etc.) have been revised — the current shape is documented in [`design.md`](design.md) and the cross-package split in [`companion-packages.md`](companion-packages.md). The motivation, preprocessor rationale, and the case against alternatives remain accurate.

---


## The problem

In real Go codebases, validation is a discipline, not a guarantee. Incoming data is validated at the edge (sometimes), then flows through dozens of functions that either re-validate defensively or silently assume it's good. When the assumption is wrong — null where a value was expected, empty string where a handle was expected, negative where a count was expected — you get a runtime error, or worse, a quiet bug.

The standard responses are all partial:

- **Runtime assertions** — catch the bug late, don't help the compiler, easy to forget.
- **Wrapper types** (`type NonEmptyString string`) — only as good as the one `New` constructor, and every conversion erases the guarantee.
- **Linters** — limited to syntactic patterns; can't reason about dataflow.
- **Generated validation code** — helps at boundaries, nothing downstream.

What refinement types give you, when they work, is a proof that flows with the value. If `x` has type `int where positive`, the compiler knows it can pass `x` wherever `positive int` is required, and *refuses* to pass it where the constraint isn't established.

Go doesn't have refinement types, and adding them to the language is out of scope for mortals. But Go has `-toolexec`, which lets us intercept compilation and do whatever we want with the source before `compile` sees it. That's enough.

## The idea

`proven` is a toolexec-driven static checker and preprocessor for Go. It scans your module for constraint markers, proves them where it can, rewrites the AST where it needs to inject guards, and lets the Go compiler take over for the rest. To users, it looks like `go build` / `go test`, just with `GOFLAGS=-toolexec=proven` set.

### Annotating values

Three annotation styles, chosen to survive inside stock Go syntax:

1. **Typed markers** — generic phantom types that ride inside Go's type system.

   ```go
   type Positive struct{}
   func (Positive) Check(x int) bool { return x > 0 }

   func Transfer(amount proven.Refined[Positive, int]) { ... }

   // Call site:
   Transfer(proven.Attest[Positive](userInput))   // checker must prove userInput > 0
   Transfer(proven.TrustMe[Positive](userInput))  // explicit runtime guard injected
   ```

   `proven.Refined[P, T]` is a zero-cost wrapper that the checker treats as "a `T` for which `P` has been established." Passing a raw `T` where `Refined[P, T]` is required fails the build unless the checker can prove `P(x)`.

2. **Doc-comment predicates** — when you want to attach a constraint to an existing function parameter without changing its type.

   ```go
   // proven:where amount > 0
   // proven:where len(note) <= 280
   func Transfer(amount int, note string) { ... }
   ```

   The toolexec pass parses these, synthesizes the matching `Refined[…]` signatures internally, and propagates the obligation to every call site.

3. **Struct tags** — for deserialization boundaries.

   ```go
   type TransferRequest struct {
       Amount int    `proven:"positive"`
       Note   string `proven:"len<=280"`
   }
   ```

   At sites marked as boundaries (see below), these become runtime checks; downstream, the values carry the refined type.

### The checker

At the toolexec boundary, before the Go compiler sees the code, `proven` runs a module-wide pass:

- **Collect obligations.** Every call site that passes a value into a refined parameter is an obligation to discharge.
- **Collect facts.** Literal values (`42`), comparisons in preceding `if` branches, results of already-proven functions, type guards, and `len(...)` relations all contribute facts to a flow-sensitive environment.
- **Discharge obligations.** For each obligation, check whether the accumulated facts entail the predicate. Simple predicates (inequalities, emptiness, length bounds, regex matches against literals) dispatch through a small rule set; general predicates fall through to an SMT backend (likely Z3 via CGO, or a pure-Go subset for the common case).
- **Emit or fail.** If every obligation discharges, the pass is a no-op — the compiler gets the original AST plus an empty marker file. If any obligation fails, the pass fails the build with a pointed error: *"cannot prove `amount > 0` at call site X; consider `proven.Attest` with a preceding check, or `proven.TrustMe` to accept a runtime guard."*

### Boundaries

Boundaries are the explicit "the data came from outside" points: HTTP handler arguments, `json.Unmarshal` results, database rows. At boundaries, the checker doesn't try to prove anything — it injects a runtime check that either refines the value (turning `int` into `Refined[Positive, int]`) or rejects the input. Downstream code then benefits from the static guarantee.

A function is a boundary if it's marked `// proven:boundary`, or if its input type has `proven:"..."` tags (deserialization target).

### Comptime

The same pass that proves constraints can evaluate pure expressions and substitute literals. A `proven.Const(expr)` call, where `expr` uses only pure functions and compile-time-known inputs, is replaced by its computed value during the pass. This gives you Zig-style `comptime` for the subset of Go that's side-effect-free — enough for configuration tables, lookup structures, and generated constants.

```go
var primes = proven.Const(sieve(10_000))  // computed at build time, baked into the binary
```

## Why toolexec

The alternatives all have worse tradeoffs:

- **A real language fork.** Enormous maintenance burden; nobody uses forks.
- **`go generate`.** Requires users to remember to run it; output must be committed; can't see across packages cleanly; can't synthesize new parameter types.
- **An external CLI.** Same problem — it's out of band, and `go build` doesn't know about it.
- **A full analyzer + external rewriter.** Two-step workflow, easy to skip the rewrite step.

`-toolexec` sits between `go build` and the underlying compile/link tools. It sees every package in the build graph in dependency order, can read and modify source before compilation, and participates in the build cache naturally. `rewire` already proved that this is tractable for a non-trivial transformation (AST rewriting for mocks). `proven` reuses that shape.

## Expressive ceiling

We cannot change Go's grammar. That means:

- Constraints ride on generic type parameters, doc comments, or struct tags — not new syntax.
- Error messages will sometimes refer to synthesized types (`Refined[Positive, int]`) that users didn't write. The checker should translate these back to source locations in its output.
- IDE integration is limited to what `gopls` understands. `Refined[P, T]` is plain Go generics, so autocomplete works; predicate doc comments are invisible to `gopls` (a separate language-server plugin is the long-term fix).
- The proof engine is where the ambition lives. Start with the subset that covers 90% of real validation (non-negative, non-empty, bounded length, regex against literal, nil-free), then extend.

## Where this differs from fl

`fl` was a greenfield language design — it could put `where` clauses anywhere. `proven` accepts Go's constraints and finds the largest useful subset of refinement typing that fits inside them. Less elegant, vastly more useful.

## Where this reuses rewire

The toolexec infrastructure — compile-invocation detection, per-package scanning, AST rewriting, build-cache interaction — is the same shape as `rewire`'s. The scanner targets different call patterns (`proven.Refined`, `proven.Attest`, doc comments, struct tags) and the rewriter does different work (synthesizing wrapper types and injecting guards vs. rewriting function bodies), but the plumbing carries over.

## Open questions

- **SMT cost at scale.** Per-package incremental checking is fine; full-module proofs on a cold cache could be slow. Need to profile realistic workloads early.
- **Cross-package obligations.** When package A calls into package B's refined parameter, A's package needs to see B's predicate. Either we require predicates to be in the same package (too restrictive), or we emit a small "constraint summary" sidecar per package that the checker reads like a signature file. Leaning toward the latter.
- **Generics and predicates.** `Refined[P, T]` composes with generics, but stating constraints that relate type parameters (`Refined[LenEq[N], []T]`) pushes into dependent-type territory. Deliberately out of scope for v1.
- **IDE diagnostics.** Without a language-server plugin, errors appear only at build time. Acceptable for v1, painful long-term.

## Minimum viable slice

1. Public API: `Refined[P, T]`, `Attest[P]`, `TrustMe[P]`, `Const`.
2. Toolexec binary that intercepts `compile`, scans for `proven.*` references, runs a trivial checker (only literal-vs-predicate), rewrites to inject guards for `TrustMe`, rejects the build for unproven `Attest`.
3. One built-in predicate (`Positive`) and one example module that uses it end-to-end.
4. Benchmark harness measuring overhead vs. stock `go build`.

Everything else — SMT, doc-comment predicates, struct-tag boundaries, `Const`, cross-package summaries — layers on after the slice works.
