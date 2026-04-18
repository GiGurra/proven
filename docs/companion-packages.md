# Companion packages: proven, prove, infer

Three cooperating packages, each covering a distinct class of constraint problem. `proven` is what's currently implemented; `prove` and `infer` are future scope, sketched here so the package boundaries are understood up-front and not accidentally collapsed back into one.

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

## `infer` — compile-time deduction

Two capabilities, unified under the "compile-time deduction" theme.

### Inference rules — now

Declare that one predicate implies another, optionally under a context. The proven preprocessor consumes these rules at scan time and uses them to discharge obligations without the caller having to re-prove a stronger predicate from scratch:

```go
// At package scope — picked up by the preprocessor.
var _ = infer.From(isSmallPositive).To(isPositive)
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
```

Reads left-to-right as the logical statement: *"from this premise, [given this context,] we conclude this."* Rules are **trusted** — the preprocessor does not symbolically verify that the declared implication actually holds. A future `infertest.Verify` helper would property-test rules on sample inputs to catch declared-but-false rules during development.

Implemented: `pkg/infer/infer.go` (runtime stubs).

### Compile-time evaluation (`comptime`) — future

Zig-style `comptime` for the pure subset of Go. Evaluate an expression at build time, substitute the resulting literal, omit the evaluation from the binary:

```go
var primes = infer.Const(sieve(10_000))
```

Under the preprocessor: `sieve(10_000)` runs during compilation; the result is emitted as a Go literal baked into the binary. Startup cost zero.

Without the preprocessor: `infer.Const` is the identity function; the expression evaluates at package-init time.

Purity is verified by the preprocessor. Impure operations (I/O, unknown functions) fail the build at the first impure operation.

**Use for:** lookup tables, configuration constants derived by computation, code generation that would otherwise live in `go generate`.

(The original `concept.md` placed `Const` under `proven.Const`. It relocates here; `proven` is reserved for contract-style assertions.)

### Why one package

Both features are "what we deduce at build time". Inference rules deduce *relationships between predicates*; `Const` deduces *values of expressions*. They share the same preprocessor pass structure and the same trust-the-declarer convention, so bundling them under `infer` keeps the mental model narrow.

## Why three packages

- **Distinct intent.** `proven.That` in a body says "caller must prove this"; `prove.Check` says "this just came from outside, validate it now"; `infer.Const` says "compute this at build time." Conflating them behind a single API obscures which is happening.
- **Distinct runtime semantics.** `proven.That` panics on violation; `prove.Check` returns errors; `infer.Const` has no runtime counterpart when the preprocessor runs. Three distinct call shapes are cleaner than one overloaded one.
- **Distinct preprocessor responsibilities.** The `proven` pass discharges via flow analysis; the `prove` pass runs at runtime (no preprocessor action at call sites, though the preprocessor does propagate proved facts downstream from `prove.Check` / `prove.Must` returns); the `infer` pass evaluates expressions and substitutes literals. Three passes, each focused.

The three packages share the preprocessor infrastructure (toolexec entry, per-package scan, AST walker, diagnostic emitter) but expose targeted APIs for each problem class. Users import only what they need.

## Current state

| Package | Status |
|---------|--------|
| `proven` | Runtime stubs implemented in `pkg/proven/`. Preprocessor is future work. |
| `prove`  | Not yet implemented. This document captures the intent. |
| `infer`  | Inference rules (`From(p).To(q)` / `From(p).Given(c).To(q)`) implemented as runtime stubs in `pkg/infer/`. Compile-time evaluation (`infer.Const`) still future. Preprocessor consumption pending. |

When the preprocessor lands, the three packages become siblings under `pkg/`, each with their runtime stubs plus the shared preprocessor pass-hooks.
