# Proof subsumption

> **Status: superseded.** This document described proof-expression
> subsumption algebra for the original `Refined[P, T]` design. The
> current design (see [`design.md`](design.md)) handles cross-predicate
> implication via user-declared rules in `pkg/infer`
> (`infer.From(p).To(q)`, optionally with `.Given(context)`). Those
> rules form a simple implication graph the preprocessor walks; there
> is no expression-level subsumption algebra. This file is kept as
> reference for how the problem was originally framed.

---


The proof-as-type-parameter design means Go's type checker sees `Refined[And[A, B, C], int]` and `Refined[And[A, B], int]` as unrelated types. Without intervention, a caller can only pass a proof that matches the parameter's declared proof *exactly in syntax*.

This is intolerable for real use. Internal refactors that add one predicate to a function signature would break every caller. Reordering `And[A, B]` to `And[B, A]` would change the type. The project is dead before it starts if we don't handle this.

The preprocessor resolves it by normalizing proof expressions and applying subsumption rules at call-site boundaries, rewriting arguments as zero-cost casts when the caller's proof entails the parameter's requirement.

## Levels of sophistication

### Level 1 — Normalization

Treat proof expressions as a small algebra and canonicalize:

- **Commutativity.** `And[A, B]` ≡ `And[B, A]`. `Or` likewise.
- **Associativity.** `And[A, And[B, C]]` ≡ `And[And[A, B], C]` ≡ `{A, B, C}`.
- **Idempotence.** `And[A, A]` ≡ `A`.
- **Double negation.** `Not[Not[A]]` ≡ `A`.

Flatten `And`/`Or` to sets; canonicalize atom order (alphabetical by fully-qualified type name). Two proofs are equal iff their canonical forms match.

### Level 2 — Set inclusion

A conjunction can be weakened: `And[A, B, C]` satisfies `And[A, B]`. After normalization, this is set containment over the conjunct atoms.

Dually, `Or[A, B]` satisfies `Or[A, B, C]` — adding alternatives weakens a disjunction.

### Level 3 — Full entailment

Arbitrary boolean combinations plus user-declared implications:

- `SmallPositive ⇒ Positive` declared once, honored at every subsumption boundary.
- De Morgan rewrites, distribution.
- Transitive implication chains.

This is SAT-lite territory. Useful but not urgent.

## v1 scope

Levels 1 and 2. This covers the practical UX — users can reorder combinators, add predicates without breaking callers — without committing to a logic engine.

Level 3 is deferred. The syntax for declaring predicate implications is a future design question; candidate syntaxes include marker methods (`func (SmallPositive) Implies() Positive`) and doc-comment declarations.

## Mechanics

### In (A) — `Refined[P, T]`

At the call site:

```go
// f expects Refined[And[A, B], T]; x has type Refined[And[A, B, C], T].
f(x)
```

Go's type checker rejects this — the instantiated types do not match. The preprocessor intervenes before `compile` runs:

1. Scan the call.
2. Canonicalize both proof expressions.
3. Verify caller's proof entails parameter's (via Level 1 + Level 2 rules).
4. Rewrite the argument to an explicit zero-cost cast:
   ```go
   f(proven.Refined[proven.And[A, B], T]{v: x.Unwrap()})
   ```
5. `compile` sees a type-matched call.

The cast compiles to a direct field read plus a struct literal — both zero-cost. No runtime overhead; only syntactic.

The user-facing form of this cast — when it needs to be written explicitly for IDE reasons — is `proven.Weaken[Q](x)`. See `docs/ide-integration.md`.

### In (C) — struct embedding

Go structs are nominally typed. `UserID{Positive, V int}` is not a `Customer{Positive, V int}` even when the field sets happen to match. Subsumption in (C) applies only when:

- The struct type is the same on both sides of the call.
- The proof-field set of the caller's value is a superset of the parameter's requirement.

The cross-struct case is not a proof question — it is a domain question. For those, the user writes an explicit constructor.

So: (A) gets rich subsumption; (C) gets near-exact-match, which is appropriate for domain entities where implicit subtyping would mask meaningful distinctions (a `PremiumUserID` is not silently a `UserID`, even if the proofs align).

## Predicate-level subtyping (Level 3, deferred)

A separate question from proof-expression subsumption. If the user declares that `SmallPositive ⇒ Positive`, then a value of `Refined[SmallPositive, int]` should satisfy a parameter of `Refined[Positive, int]`. This is subtyping at the *atom* level, not the expression level.

Implementation sketch for the future:

- User declares implications via a marker method or doc comment on the predicate type.
- Preprocessor builds an implication graph during scan.
- Subsumption check at call sites walks the graph (transitive closure) when direct set inclusion fails.

Deliberately out of scope for v1.

## Interaction with IDE tooling

The preprocessor's rewrite is invisible to `gopls`. Without intervention, the IDE flags every subsumption-requiring call as a type error even though the build succeeds. That is a separate-enough problem to get its own document — see `docs/ide-integration.md`.
