# Relations between values

The v1 predicate shape is strictly unary: `func(T) bool`. A predicate constrains one value. Many interesting properties are *between* values: "controller `c` has authorized executor `e`", "session `s` is owned by user `u`", "the transaction's source account `src` and destination `dst` belong to the same bank". A unary predicate cannot express these directly.

This document captures the exploration we did before shipping Phase 7.5 and the decision we landed on. The intent is that a future reader who wants to revisit the design finds everything they need to pick up from where we paused — the three approaches we considered, the AST shapes each produces, the tradeoffs, and the reasons we chose what we chose.

## The three approaches

We compared three ways to express a 3-subject authorization relation across (`Session`, `User`, `Resource`):

### 1. Tuple-as-subject (chosen for v1)

Define a domain struct that packs the subjects of the relation, and write the predicate as unary over that struct.

```go
type AuthCtx struct {
    S Session
    U User
    R Resource
}

func CanModify(a AuthCtx) bool { /* ... */ }

func ModifyResource(a AuthCtx) {
    proven.That(a, CanModify)
    // use a.S, a.U, a.R
}

func handler(s Session, u User, r Resource) {
    a := AuthCtx{S: s, U: u, R: r}
    if CanModify(a) {
        ModifyResource(a) // CanModify(a) is proven; call accepted
    }
}
```

### 2. Currying (deferred)

Model a 3-ary relation as a curry function that captures the first N−1 subjects and returns a unary predicate over the last.

```go
func CanModifyFn(s Session, u User) func(Resource) bool {
    return func(r Resource) bool { /* ... */ }
}

func ModifyResource(s Session, u User, r Resource) {
    proven.That(r, CanModifyFn(s, u))
}

func handler(s Session, u User, r Resource) {
    if CanModifyFn(s, u)(r) {
        ModifyResource(s, u, r)
    }
}
```

### 3. Explicit `proven.Relation` (deferred)

A new `proven.Relation(pred, subjects...)` API for n-ary obligations. Predicate is an ordinary Go function taking N args.

```go
func CanModify(s Session, u User, r Resource) bool { /* ... */ }

func ModifyResource(s Session, u User, r Resource) {
    proven.Relation(CanModify, s, u, r)
}

func handler(s Session, u User, r Resource) {
    if CanModify(s, u, r) {
        ModifyResource(s, u, r)
    }
}
```

## What the AST looks like in each

Spiked with real Go files and `go/parser` to see what the scanner and analyzer would actually have to recognize.

**Tuple — obligation `proven.That(a, CanModify)` and guard `if CanModify(a)`:**

```
CallExpr
  SelectorExpr proven.That
  arg[0]: Ident "a"
  arg[1]: Ident "CanModify"

CallExpr
  Ident "CanModify"
  arg[0]: Ident "a"
```

These are exactly the shapes the existing analyzer already handles. No new case needed anywhere.

**Currying — obligation `proven.That(r, CanModifyFn(s, u))` and guard `if CanModifyFn(s, u)(r)`:**

```
CallExpr
  SelectorExpr proven.That
  arg[0]: Ident "r"
  arg[1]: CallExpr                 ← predicate expression is now a CallExpr
    Ident "CanModifyFn"
    arg[0]: Ident "s"              ← captured args
    arg[1]: Ident "u"

CallExpr                           ← guard is a call-of-a-call
  CallExpr
    Ident "CanModifyFn"
    arg[0]: Ident "s"
    arg[1]: Ident "u"
  arg[0]: Ident "r"                ← subject
```

The scanner's `resolvePredicate` today accepts `Ident` / `SelectorExpr`; it would need a `CallExpr` case that extracts the curry function's name plus the captured args (as parameter-index references on the enclosing function). The analyzer's `callAsPredicate` today requires `len(call.Args) == 1` and rejects `CallExpr` as function; it would need to recognize the nested-call shape.

**Explicit Relation — obligation `proven.Relation(CanModify, s, u, r)` and guard `if CanModify(s, u, r)`:**

```
CallExpr
  SelectorExpr proven.Relation     ← new matcher
  arg[0]: Ident "CanModify"
  arg[1]: Ident "s"
  arg[2]: Ident "u"
  arg[3]: Ident "r"

CallExpr
  Ident "CanModify"
  arg[0]: Ident "s"
  arg[1]: Ident "u"
  arg[2]: Ident "r"                ← n-ary guard
```

`matchProvenCall` needs a new case for `Relation`. `callAsPredicate` must return `[]string` subject names, not a single `string`. `Fact` must carry `Vars []string`, not a single `Var`.

## Comparison

| Axis | Tuple | Currying | `proven.Relation` |
|---|---|---|---|
| New API in `pkg/proven` | none | none | yes — `Relation(pred any, args ...any)` + test-mode reflection |
| Change to `Fact` / `Predicate` struct | none | `Predicate` grows `CapturedArgs` | `Fact` grows `Vars []string` |
| Scanner change | none | one new case in `resolvePredicate` (`CallExpr` as predicate) | new `Relation` matcher, n-ary arg handling |
| Analyzer change | none | 2-level `callAsPredicate`, captured-arg substitution at call sites | n-ary `callAsPredicate`, `Vars` substitution at call sites |
| Rewriter change | none | erase curry-call obligation shapes | erase `Relation` obligation shapes |
| Sidecar (Phase 6) change | none | `Predicate` serialization grows captured-arg slots | `FuncSummary` grows relational-obligation entries with subject-param-index lists |
| Inference rules | identical to unary; full support for `infer.From(...).[Given(...).]To(...)` | clean for same-captured-arg-shape chains (`FromFn(P).To(Q)`); awkward for multi-premise rules with differing captured-arg shapes (no natural binder at package scope) | needs either string binders (`Premise(P, "u", "s"), ...`), or body-scan (parse a function literal as the rule) |
| Unary still works unchanged | yes | yes (empty captured args) | yes (`len(Vars) == 1`) |
| Type safety at call sites | full — Go types check the tuple and the predicate | full — each curry step typechecks | partial — `pred any`, `...any` at obligation site (predicate itself still typed at definition) |
| Arity | unbounded (struct fields) | unbounded (curry depth) | unbounded (`...any`) |
| User ergonomics | must design a domain struct per relation; access members via `.Field` | native Go syntax `f(a)(b)`, unusual but legal | dedicated API — discoverable; single call site per obligation |
| Purity hazard | none | yes — user-captured args must be deterministic, or analyzer compile-time reasoning diverges from runtime | none |

## Why tuple wins v1

Two things decided it:

**Zero preprocessor changes.** The tuple spike parses to the AST shapes the existing analyzer already handles. Every existing capability — guards, early-return, `proven.Returns`, `prove.That`/`Must`, `trust.That`, `infer.From/Given/To`, the Phase 6 cross-package sidecar, the Phase 5b erasure — applies unchanged. We ship v1 Phase 7.5 as documentation plus a couple of fixtures verifying it works; zero new code paths.

**Optionality preserved.** Shipping tuple-first does not commit us. Currying and `proven.Relation` remain fully open as later additions if tuple-wrapping proves to have pain points. In contrast, shipping either of the other two first would commit us to a new surface API (`Relation`) or a new Predicate identity shape (currying captures) before we have user-driven evidence that they earn their complexity.

The cost is real but bounded: **users must design domain types for their relations**. `AuthCtx`, `TransactionScope`, `SessionActivity`, etc. — structs that pack participating subjects. Member access is `a.S` / `a.U` / `a.R` rather than three separate parameters. In our judgment this is often a *feature* — forcing users to name their relations pushes design clarity — but it is also a new discipline that will surprise some authors.

## When to revisit

Reopen the design if one of these surfaces in real user code:

1. **Users consistently wrap the same subjects into the same ad-hoc tuple type at every call site.** If every call to `ModifyResource(s, u, r)` is preceded by `a := AuthCtx{s, u, r}` for no purpose other than satisfying the contract, the tuple is noise, not modeling. That is the signal to prefer `proven.Relation` (or currying).

2. **The subjects cannot be grouped at source** — the relation crosses function boundaries the user does not own (interface method receivers, callbacks with fixed signatures). A tuple helper around an immutable API shape is awkward.

3. **Compositional inference between relations becomes load-bearing.** Tuple predicates compose via unary `infer.From(...).To(...)` rules over the struct subject, but each struct's field shape is a commitment. If users want to say "for all `(u, r)` pairs such that `OwnedBy(u, r)` and `SessionOf(u, s)`, `CanModify` holds over `(s, u, r)`" — that's a compositional relational rule with subject rebinding. Tuples can't express it without an explicit "combine" function that packs one tuple into another. `proven.Relation` with a body-scanned rule API handles this more directly.

Of the two deferred approaches, my current read:

- **Currying is better for obligations and guards** — smaller surface area, no new API.
- **`proven.Relation` is better for multi-premise inference rules with subject rebinding** — cleaner binder story, either via string-named subjects or via body-scanning a function literal.

A hybrid (currying for obligations, `Relation` only when compositional inference needs it) is possible but adds two independent feature axes to understand. If we end up needing both, collapsing to `Relation` everywhere is probably cleaner than keeping two parallel shapes.

## What tuple-first actually ships

- No code changes to the scanner, analyzer, rewriter, sidecar, or runtime packages.
- `example/` grows a relation example demonstrating the pattern end-to-end with wiring tests.
- `testdata/cases/` adds two fixtures proving the pattern works through the existing machinery: `tuple_relation_guard_discharges_ok` (guard discharges the obligation) and `tuple_relation_unguarded_fails` (no guard produces the expected diagnostic).
- `docs/design.md` gains a "Relations between values" section pointing at this document.

## Open questions left for later

- Can we teach the preprocessor to recognize *trivial* tuple wrapping at call sites and auto-discharge it? "If the caller builds `AuthCtx{S: s, U: u, R: r}` from idents that each individually satisfy some predicate, the predicate on the tuple is that of the tuple itself" — but this requires reasoning about struct-field preconditions, which is a can we have not opened.
- Does `trust.That` on a tuple compose cleanly? Should be yes — `v := trust.That(a, CanModify)` should work the same as any other unary trust injection. Worth a fixture.
- Is there a clean doc-generation story for "this function asserts `CanModify(a)` where a's fields mean `(s, u, r)`"? The signature doesn't tell the reader which subjects participate. Probably belongs to the godoc-integration open question in `docs/design.md`.
