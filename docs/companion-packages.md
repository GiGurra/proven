# Companion packages: proven, prove, infer, trust

Four cooperating packages, each covering a distinct class of constraint problem. Each package is named for its *mechanism* so the call site reads as what the program actually does: static proof (`proven`), runtime check (`prove`), declared implication (`infer`), or unverified assertion (`trust`).

## `proven` — compile-time contracts with runtime fallback

Already implemented. Precondition/postcondition assertions declared inside function bodies:

```go
func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty)
    // ... body ...
}
```

Under the preprocessor: each caller must discharge these via flow analysis; calls that succeed erase the `That` to zero runtime cost; calls that fail break the build. Without the preprocessor: the `That` call runs as a plain runtime contract check.

**Use for:** internal APIs where callers are expected to have already proven their inputs. Proofs flow through call chains: a value returned via `proven.Returns` carries its postcondition into the next call.

See [`design.md`](design.md) for the full specification.

## `prove` — runtime validation at system boundaries

For data crossing an external boundary: HTTP bodies, decoded JSON, CLI arguments, database rows, parsed files. Static proof is impossible at the boundary by definition — the value's properties are unknown until runtime. `prove` provides explicit runtime validators that establish proofs from raw input:

```go
// Return an error — for HTTP handlers, decoders, etc.
func (v T, preds ...func(T) bool) error
    // prove.Check

// Panic on violation — for startup paths where failure is fatal.
func (v T, preds ...func(T) bool) T
    // prove.Must
```

After a successful `prove.Check` or `prove.Must`, the value is "proven" in the same sense as `proven.That`: the preprocessor treats it as satisfying the relevant predicate downstream. `prove` is how external data *becomes* proven.

Sketch usage:

```go
func handleTransfer(r *http.Request) error {
    var req TransferRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        return err
    }
    if err := prove.Check(req.Amount, isPositive); err != nil {
        return err
    }
    // From here on, req.Amount is proven isPositive — proven.That calls
    // downstream discharge for free.
    return Transfer(req.Amount, req.Note, req.Currency)
}
```

**Use for:** HTTP handlers, JSON/XML/protobuf decoders, CLI arg parsing, config loaders, database row mapping — any place raw external data enters the program.

## `trust` — the "trust me" escape hatch

The one part of proven where the claim is yours, not the compiler's. Every other discharge path gets the compiler (or a runtime check) behind the fact; `trust.That` is the single call site where **you** sign for it instead. Used for values that have already been validated by a mechanism the analyzer cannot see — a JSON schema validator, a database CHECK constraint, an upstream generated decoder, an audited business-logic invariant — and where a redundant runtime re-check would be cost for no safety gain. Call-site naming makes the mechanism obvious:

```go
// prove.That — runtime check, error return
v, err := prove.That(raw, isPositive)

// prove.Must — runtime check, panic on fail
v := prove.Must(raw, isPositive)

// trust.That — NO runtime check, asserts the fact, propagates it locally
v := trust.That(raw, isPositive)

// trust.Returns — NO runtime check, asserts the fact AND advertises it
// as the enclosing function's postcondition (visible to every caller
// across packages). Pairs with proven.Returns for the callee-advertised
// postcondition story, but without the site verification proven.Returns
// enforces.
return trust.Returns(42, isPositive)
```

Under the preprocessor: the call is erased and each listed predicate becomes a flow fact on the LHS, parallel to `prove.Must`'s analyzer handling but without the runtime side-effect. Without the preprocessor: the call is an identity pass-through at no runtime cost — a program that uses only `trust` links and runs without the toolexec pipeline.

Distinct from `proven.Returns`:

- `proven.Returns` wraps a return value, advertising a **function-level postcondition** via the package's summary — every caller across packages sees it.
- `trust.That` is **local** — it injects facts into the enclosing function's flow state, invisible to callers. If every caller should see the fact, use `proven.Returns`.

**Use for:** sites where runtime re-validation would duplicate an earlier check the caller has already made, the cost is meaningful, and the programmer is willing to own the correctness of the assertion. Do not reach for `trust` when `prove.Must` would do — a free runtime check is usually worth it.

Implemented: `pkg/trust/trust.go`.

## `infer` — compile-time deduction

Two capabilities, unified under the "compile-time deduction" theme.

### Inference rules — now

Declare that one predicate implies another, optionally under a context. The proven preprocessor consumes these rules at scan time and uses them to discharge obligations without the caller having to re-prove a stronger predicate from scratch:

```go
// At package scope — picked up by the preprocessor.
var _ = infer.From(isSmallPositive).To(isPositive)
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

// Every slot is variadic and AND-composes, matching proven.That /
// prove.That / trust.That. A multi-argument From requires every
// premise; a multi-argument To asserts every conclusion.
var _ = infer.From(isEven, isPositive).To(isNonNeg, isNonZero)

// Inline proven.And also works in every slot: the scanner flattens
// it into its leaves on ingest, so this reads the same as the rule
// above.
var _ = infer.From(proven.And(isEven, isPositive)).To(isNonNeg, isNonZero)
```

Reads left-to-right as the logical statement: *"from these premises, [given this context,] we conclude these."* Rules are **trusted** — the preprocessor does not symbolically verify that the declared implication actually holds. `pkg/infertest.Verify(t, rule, samples...)` (implemented) offers a cheap property-test: it evaluates the rule's premise, Given, and conclusion on each sample and reports a failure whenever premise and Given both hold but conclusion does not. A stricter `VerifyApplies` additionally fails if no sample triggered the premise at all — useful when the caller is expected to supply a coverage-relevant sample set rather than relying on silent vacuous-truth.

Implemented: `pkg/infer/infer.go` (runtime stubs); `pkg/infertest/infertest.go` (`Verify`, `VerifyApplies`).

### Compile-time evaluation — not in scope here

Earlier iterations of this document proposed a Zig-style `infer.Const` for evaluating pure expressions at build time. When we sat down to design it we concluded it belongs in a separate project: its mechanisms, use cases, and hard problems share almost nothing with contract-system predicates and their discharge. The full exploration — candidate execution models, real code spikes, final recommendation — lives in [`comptime.md`](comptime.md). The `infer` package in this project is specifically about predicate-implication declarations, nothing else.

## Why four packages

- **Distinct intent.** `proven.That` says "caller must prove this"; `prove.That` / `prove.Must` say "this just came from outside, validate it now"; `trust.That` says "I've already validated this by other means, take my word for it"; `infer.From(p).To(q)` says "these predicates relate." Each verb carries its own semantics; conflating them behind a single API obscures which is happening.
- **Distinct runtime semantics.** `proven.That` is erased by the preprocessor and has a no-op runtime stub; `prove.That` runs at runtime and returns an error; `prove.Must` runs at runtime and panics on failure; `trust.That` never runs any check; `infer.From(p).To(q)` is consumed by the preprocessor as a declaration with no runtime behavior beyond holding its predicates for `infertest.Verify`.
- **Distinct preprocessor responsibilities.** The `proven` pass discharges via flow analysis; the `prove` pass injects facts at successful-check branches (runtime code is unchanged); the `trust` pass injects facts unconditionally and erases the call; the `infer` pass consumes declared rules for backward-chaining. Four passes, each focused.

All four packages share the preprocessor infrastructure (toolexec entry, per-package scan, AST walker, diagnostic emitter) but expose targeted APIs for each problem class. Users import only what they need.

## Current state

| Package | Status |
|---------|--------|
| `proven` | `That` / `Returns` / `And` / `Or` / `Not` implemented; preprocessor discharges call sites (Phases 1–5) and erases cleared calls. |
| `prove`  | `That(v, preds...) (T, error)` and `Must(v, preds...) T` implemented; preprocessor propagates post-check facts (Phase 3). |
| `trust`  | `That(v, preds...) T` implemented in `pkg/trust/`; preprocessor injects facts at every call site and erases the call (Phase 7). |
| `infer`  | Inference rules (`From(p).To(q)` / `From(p).Given(c).To(q)`) implemented; preprocessor uses them for backward-chaining discharge (Phase 4). |
