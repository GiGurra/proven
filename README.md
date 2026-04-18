# proven

[![CI Status](https://github.com/GiGurra/proven/actions/workflows/ci.yml/badge.svg)](https://github.com/GiGurra/proven/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GiGurra/proven)](https://goreportcard.com/report/github.com/GiGurra/proven)
[![Docs](https://img.shields.io/badge/docs-gigurra.github.io%2Fproven-blue)](https://gigurra.github.io/proven/)

> **Experimental** — APIs and internals may change. Use at your own risk.

Compile-time contracts for Go. Declare what must hold inside a function body; the preprocessor proves it at every call site, or fails the build.

```go
// Declare a precondition inside the function body:
func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty, maxLen280)
    // ... body ...
}

// Declare a postcondition on a return value. The value must already carry
// the predicate as a fact at the Returns site — here the function's own
// declared precondition does that work:
func Normalize(x int) int {
    proven.That(x, isPositive)
    return proven.Returns(x, isPositive)
}

// For a literal or computed expression the analyzer can't reason about,
// trust.Returns vouches-and-advertises in one call:
func DefaultUserID() int {
    return trust.Returns(42, isPositive) // programmer: "42 is obviously positive"
}

// Declare an implication once; the preprocessor uses it to discharge obligations:
var _ = infer.From(isSmallPositive).To(isPositive)

// Boundary-validate external data and carry the proof forward:
v, err := prove.That(raw, isPositive)     // runtime check, returns error
v := prove.Must(raw, isPositive)          // runtime check, panics on fail
v := trust.That(raw, isPositive)          // no runtime check — programmer takes responsibility

// Property-test an inference rule on sample inputs:
infertest.Verify(t, myRule, 1, 5, 99, -3, 0)
```

Signatures stay plain Go. Predicates are ordinary `func(T) bool`. No wrapper types, no generics ceremony, no struct decorations, no codegen. `gopls` and `go vet` see ordinary code, so IDE checking stays green — but building without the preprocessor fails loudly at link time, so you cannot silently ship code that bypasses the contract system.

**The point isn't more validation. It's to stop having to remember.** Systems grow, defensive re-validation accretes at every layer, and nobody remembers why a particular check was ever needed. `proven` shifts the remembering onto the compiler: a function declares once what must hold, the preprocessor proves it at build time, discharged obligations are erased, and un-discharged ones fail the build.

## Quick start

```bash
# Install the preprocessor binary
go install github.com/GiGurra/proven/cmd/proven@latest

# Add the runtime packages to your module
go get github.com/GiGurra/proven

# Build or test with the preprocessor active
GOFLAGS="-toolexec=proven" go build ./...
```

See [Setup](#setup--ide-and-cache-configuration) below for the split-cache setup that keeps toolexec and non-toolexec builds from contaminating each other. Full documentation: **[gigurra.github.io/proven](https://gigurra.github.io/proven/)**.

<details>
<summary><strong>Multiple predicates and combinators</strong></summary>

`proven.That` and `proven.Returns` are variadic. Multiple predicates AND-compose:

```go
func Charge(amount int, note string) error {
    proven.That(amount, isPositive, lessThan1000)
    proven.That(note, isNonEmpty, maxLen280)
    return nil
}
```

For OR composition, negation, or a reusable composite predicate, use `proven.And` / `proven.Or` / `proven.Not`. They return plain `func(T) bool`, so they compose freely and can be stored in package-level variables:

```go
var validCurrency = proven.Or(isUSD, isEUR, isGBP)
var sensibleQty   = proven.And(isPositive, lessThan1000)
var eligibleUser  = proven.Not(isBanned)

func ChargeFX(amount int, currency string, userID int) error {
    proven.That(amount,   sensibleQty)
    proven.That(currency, validCurrency)
    proven.That(userID,   eligibleUser)
    return nil
}
```

Callers of a function with `proven.Returns(v, isPositive)` as its return automatically get `isPositive` as a fact on the returned value — no re-check at the call site.

</details>

<details>
<summary><strong>Discharge patterns — how facts get established, and what happens when they aren't</strong></summary>

**The core insight:** you don't use a special "proof" API to establish a fact. The preprocessor watches the caller's ordinary control flow and treats any check it can see as a fact in the then-branch. A plain `if isPositive(x) { ... }` and a boundary-validating `prove.That(raw, isPositive)` produce **the same fact** in the analyzer — the difference is only where the value came from and whether you wanted an error return.

```go
// Path A — regular predicate call in a guard.
if isPositive(x) {
    Transfer(x, "hi")   // discharged: isPositive(x) is a fact in the then-branch
}

// Path B — prove.That for boundary-validated values, same resulting fact.
v, err := prove.That(raw, isPositive)
if err != nil {
    return err
}
Transfer(v, "hi")       // discharged: isPositive(v) is a fact on the err==nil side
```

Both discharge `proven.That(amount, isPositive)` inside `Transfer` identically. Pick whichever matches the shape of the surrounding code.

**What a missed proof looks like.** If you drop the guard, the build fails:

```go
func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    return nil
}

func main() {
    x := readUserInput()
    Transfer(x, "hi")   // no guard — build fails
}
```

```
main.go:NN:CC: proven: undischarged predicate isPositive on parameter 0 of Transfer
```

Same `file:line:col:` format Go's own compiler uses, so your editor click-through works. The diagnostic names the missing predicate, which parameter it's on, and which callee needed it — so the fix is either an explicit guard, a `prove.That` at the boundary, a `proven.Returns` postcondition from the producer, or a `trust.That` injection if you've validated it by a mechanism the analyzer can't see.

**Fact sources** the preprocessor recognizes:

```go
// 1. Preceding predicate check.
if isPositive(x) {
    Transfer(x, ...) // isPositive(x) is a fact here
}

// 2. Early-return guard.
if !isPositive(x) {
    return
}
Transfer(x, ...)     // isPositive(x) holds from here on

// 3. Conjoined guard (&&).
if isPositive(x) && x < 100 {
    Target(x)        // both facts available in the body
}

// 4. The function's own declared preconditions. Inside F's body,
//    each parameter carries the predicates F declared on it as
//    starting facts — every caller proved this at the call site,
//    so inside F the facts hold by construction.
func F(x int) {
    proven.That(x, isPositive) // precondition
    G(x)                       // G wants isPositive — discharged from seeding
}

// 5. proven.Returns postcondition.
v := DefaultUserID() // DefaultUserID advertises isPositive via Returns
Target(v)            // isPositive(v) is a fact

// 6. prove.That or prove.Must success.
v, err := prove.That(raw, isPositive)
if err != nil { return err }
Target(v)            // isPositive(v) is a fact on the err == nil side

// 7. trust.That local injection.
v := trust.That(raw, isPositive) // programmer's word, no runtime check
Target(v)                        // isPositive(v) is a fact
```

Obligations are discharged by direct fact match, by backward-chaining through declared inference rules, or by a `trust.That` injection. The proof rides along as far as you pass the value: each function along the chain declares the precondition it needs as its own `proven.That`, which — once discharged by the caller — becomes a fact inside that function's body for every downstream call that also needs it.

**Predicates must be named — with two structural carve-outs for `proven.And` and `proven.Or`.** Every predicate argument to `proven.That`, `proven.Returns`, `prove.That`, `prove.Must`, `trust.That`, and each slot in `infer.From(...).[Given(...).]To(...)` must ultimately reduce to named functions or `pkg.Name` selectors. Function literals (`proven.That(x, func(n int) bool { ... })`) fail the build with a diagnostic pointing at the offending expression — declare them as package-level vars and reference the name.

Inline `proven.And(a, b)` is accepted at obligation / fact sites: the scanner decomposes it into its leaf predicates on ingest, so `proven.That(x, proven.And(a, b))` is equivalent to `proven.That(x, a, b)`. Nested `And` flattens fully.

Inline `proven.Or(a, b)` is also accepted at `proven.That`, `proven.Returns`, `prove.That`/`prove.Must`, and `trust.That`/`trust.Returns`. An Or-obligation on a variable discharges when **any** single alternative holds as a leaf fact, OR when a structurally-matching Or-fact was planted (e.g. from `prove.That(raw, proven.Or(a, b))`). Or's arguments must be named leaves — nested combinators inside Or are v2 scope. Or inside `infer.From/Given/To` slots stays rejected: a disjunctive premise/conclusion is better expressed as one rule per branch.

Inline `proven.Not` remains rejected — it would need a negation-fact representation the analyzer does not yet carry.

This is the "no silent bypass" principle: a predicate the scanner cannot track by identity cannot be used for cross-package discharge, and accepting it would silently weaken the contract.

</details>

<details>
<summary><strong>Inference rules — discharge by implication</strong></summary>

When a caller has established predicate `P` but a callee requires predicate `Q`, declare the implication once at package scope:

```go
// Unconditional: isSmallPositive ⇒ isPositive
var _ = infer.From(isSmallPositive).To(isPositive)

// Conditional: isEven ⇒ isPositive, but only when the value is positive
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

// Every slot is variadic and AND-composes — same as proven.That.
// Multi-premise: every listed premise must hold for the rule to fire.
// Multi-conclusion: every listed conclusion follows when it does.
var _ = infer.From(isEven, isPositive).To(isNonNeg, isNonZero)
```

The preprocessor consumes these during scan and uses backward chaining to discharge obligations without re-proving from scratch. Rules are **trusted** — the preprocessor does not symbolically verify them. Declarers are responsible for soundness, and `infertest.Verify` lets you property-test rules on sample inputs:

```go
var myRule = infer.From(isSmallPositive).To(isPositive)

func TestRule(t *testing.T) {
    infertest.Verify(t, myRule, 1, 5, 99, -3, 0) // reports any counter-example
}
```

`VerifyApplies` is the stricter variant that additionally fails when no sample triggered the premise — so a sample set that passes only because the rule never applied is caught.

</details>

<details>
<summary><strong>Test-time verification — drift defense</strong></summary>

Contracts verified only at build time are fragile: nothing stops you from accidentally removing one ("why is this line here?") or weakening it (someone narrows `isPositive` to `isNonNegative` in a refactor). `proven` defends against this at *test* time.

The preprocessor erases `proven.That` at build as usual. Inside tests you can opt in to running the blocks at runtime and assert the right predicate fires on the right parameter:

```go
import "github.com/GiGurra/proven/pkg/proventest"

// Bad input: the declared predicate must reject it.
func TestTransfer_RejectsNegativeAmount(t *testing.T) {
    proventest.AssertFails(t, isPositive, func() {
        Transfer(-5, "hi") // isPositive must reject -5
    })
}

// Good input: no declared predicate should fire.
func TestTransfer_AcceptsValidInput(t *testing.T) {
    proventest.AssertPasses(t, func() {
        Transfer(5, "hi") // every declared contract along the call chain accepts
    })
}
```

`AssertFails` pins "this predicate must fire on this input" — if someone drops `proven.That(amount, isPositive)` or weakens it, the test fails with `expected isPositive to fire, got isNonNegative`. `AssertPasses` is its symmetric counterpart — "every declared contract accepts this input" — so you can lock in known-valid inputs and catch a refactor that accidentally tightens a predicate. Production still runs at zero overhead; the runtime mode is strictly additive.

There's also `proventest.AssertAnyFailure` when only the existence of a violation matters, and the raw `proventest.WithChecks(fn)` primitive if you want to compose something custom.

</details>

<details>
<summary><strong>Runtime boundary validation — <code>pkg/prove</code></strong></summary>

Data crossing an external boundary — HTTP bodies, decoded JSON, CLI arguments, database rows — cannot be proved statically. `pkg/prove` runs a runtime check and establishes the resulting fact for downstream code:

```go
import "github.com/GiGurra/proven/pkg/prove"

func handleTransfer(r *http.Request) error {
    var req TransferRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        return err
    }
    amount, err := prove.That(req.Amount, isPositive)
    if err != nil {
        return err
    }
    // From here on, amount is known isPositive — Transfer's
    // obligation discharges without a re-check.
    return Transfer(amount, req.Note)
}
```

`prove.Must(v, preds...)` is the panic-on-fail variant for startup paths where failure is fatal.

</details>

<details>
<summary><strong>The "trust me" escape hatch — <code>pkg/trust</code></strong></summary>

Every other discharge path in proven gets the compiler (or a runtime check) behind the claim. `trust.That` is the single call site where **you** stand behind it instead — no runtime check, no static proof, just your word and the git blame.

```go
import "github.com/GiGurra/proven/pkg/trust"

// v is known-positive by upstream validation (schema validator, audited
// invariant, DB CHECK constraint) — a redundant re-check would be cost
// for no safety gain.
v := trust.That(raw, isPositive)
Transfer(v, note) // isPositive discharged on the programmer's word
```

Reach for `prove.Must` first if a free runtime check would do — `trust.That` earns its keep only when re-validation would duplicate a known-correct earlier check and the cost is meaningful.

`trust.That` is **local** — the fact is injected into the enclosing function's flow state only. For a function-level postcondition every caller can see across packages, use `proven.Returns`. The scanner verifies `proven.Returns` at the call site (the value must carry the predicates as facts), so for literals and computed expressions that the analyzer cannot reason about, use `trust.Returns` — the "trust me" variant of `proven.Returns` that both asserts the fact on your word AND advertises the postcondition:

```go
func DefaultUserID() int {
    return trust.Returns(42, isPositive) // programmer: "42 is obviously positive"
}
```

Callers of `DefaultUserID` get `isPositive` as a fact on the returned value, same as if the function used `proven.Returns`. There is no site verification — the whole point of trust is that you have vouched for it.

</details>

<details>
<summary><strong>Relations between values</strong></summary>

Predicates are unary (`func(T) bool`). For properties that relate two or more values — "controller `c` has authorized executor `e`", "session `s` is owned by user `u`" — pack the participating subjects into a domain struct and write the predicate as unary over it:

```go
type AuthCtx struct {
    S Session
    U User
    R Resource
}

func canModify(a AuthCtx) bool { /* ... */ }

func modifyResource(a AuthCtx) {
    proven.That(a, canModify)
}

func handler(s Session, u User, r Resource) {
    a := AuthCtx{S: s, U: u, R: r}
    if canModify(a) {
        modifyResource(a) // discharged via the unary analyzer, no new machinery
    }
}
```

Every existing discharge pattern works unchanged. Two deferred alternatives (currying, and an explicit `proven.Relation` API) and the triggers that would reopen the decision are captured in [`docs/relations.md`](docs/relations.md).

</details>

<details>
<summary><strong>How it works</strong> — compile-time rewriting via <code>-toolexec</code></summary>

1. **Scan.** The preprocessor parses the source files the Go compiler received and builds a per-package `PackageSummary`: which parameters of which functions carry `proven.That` obligations, which return values carry `proven.Returns` postconditions, and which `infer.From(...).[Given(...).]To(...)` rules the package declares.

2. **Cross-package sidecar.** Each clean-analyzed package writes its `PackageSummary` to `<.a-dir>/_pkg_.proven.json`. Downstream compiles parse the compile's `-importcfg` and read each imported package's sidecar so cross-package obligations are visible.

3. **Analyze.** For each call site, a flow-sensitive walk of the caller decides which obligations are discharged and which are missing. Fact sources: preceding `if pred(x)`, early-return / panic guards, `&&`-conjoined conditions, `proven.Returns` postconditions, and successful `prove.That` / `prove.Must` (correctly gated on the err-check branch). A declared inference rule lets the analyzer discharge `Q` when `P` is known by backward chaining.

4. **Emit or fail.** Undischarged obligations produce a Go-standard `file:line:col: proven: undischarged predicate X on parameter N of Y` diagnostic and the build fails before the real compile runs. When every obligation is discharged, the rewriter blanks every `proven.That` / `proven.Returns` / `trust.That` call span in the source the compiler sees. Edits are length-preserving so cmd/compile error columns still match user source column-for-column.

5. **Link-time IDE gate.** `proven.That` / `proven.Returns` wrap their checks in `atCompileTime(func() { ... })`, where `atCompileTime` is declared via `//go:linkname` to the external symbol `_proven_atCompileTime`. `gopls` and `go vet` see plain Go. But `go build` / `go test` on a main or test target refuses to link without the preprocessor — which supplies the missing symbol. Forgetting the preprocessor is a loud link failure, never a silent loss of static checking.

6. **Test-time runtime mode.** `pkg/proventest` supplies the symbol for test binaries. By default it's a no-op (matches production). Inside `proventest.WithChecks(fn)`, `AssertFails(t, pred, fn)`, or `AssertPasses(t, fn)`, each `atCompileTime` block actually executes and a failing predicate panics with a structured `proven.Violation`.

Full design in [`docs/design.md`](docs/design.md).

</details>

<details>
<summary><strong>Setup</strong> — IDE and cache configuration</summary>

Go's build cache key does not include toolexec state. A `pkg/proven.a` cached from a plain `go build` (no stub) and one from a `-toolexec=proven` build (with stub) have the same key. Symptoms of mixing:

- `relocation target _proven_atCompileTime not defined` — toolexec build reused a stub-less artifact.
- `duplicated definition of symbol _proven_atCompileTime` — non-toolexec test binary (pulling in `proventest`) reused a stub-containing artifact.

Keep toolexec on a dedicated GOCACHE.

**Terminal:**

```bash
alias gobuild-proven='GOFLAGS="-toolexec=proven" GOCACHE="$HOME/.cache/proven-build" go build'
alias gotest-proven='GOFLAGS="-toolexec=proven" GOCACHE="$HOME/.cache/proven-build"  go test'
```

**GoLand:** Run → Edit Configurations → Templates → Go Test → Environment variables:

```
GOFLAGS=-toolexec=proven
GOCACHE=/Users/<you>/.cache/proven-build
```

**VS Code (settings.json):**

```json
"go.buildEnvVars": {
    "GOFLAGS": "-toolexec=proven",
    "GOCACHE": "${env:HOME}/.cache/proven-build"
},
"go.testEnvVars": {
    "GOFLAGS": "-toolexec=proven",
    "GOCACHE": "${env:HOME}/.cache/proven-build"
}
```

Clean the proven cache specifically:

```bash
GOCACHE="$HOME/.cache/proven-build" go clean -cache
```

</details>

<details>
<summary><strong>Packages</strong></summary>

| Package | Purpose |
|---|---|
| `pkg/proven` | Contract assertions: `That`, `Returns`, `And` / `Or` / `Not`. |
| `pkg/prove` | Runtime boundary validators: `That(v, preds) (T, error)`, `Must(v, preds) T`. |
| `pkg/trust` | Local fact injection without runtime check: `That(v, preds) T`. Plus `Returns(v, preds) T` — combines `trust.That` with `proven.Returns` (no-check postcondition advertisement). |
| `pkg/infer` | Inference-rule builder: `From(p).[Given(c).]To(q)`. |
| `pkg/infertest` | Property-test inference rules: `Verify(t, rule, samples...)`, `VerifyApplies`. |
| `pkg/proventest` | Test-time linker stub + `WithChecks` / `AssertFails` / `AssertPasses` / `AssertAnyFailure`. |
| `cmd/proven` | Toolexec preprocessor binary. |

</details>

<details>
<summary><strong>Status</strong></summary>

Phases 1–8 are done: stub injection, per-package obligation scan, flow-sensitive discharge with five fact sources, inference-rule backward chaining, wiring + diagnostics, zero-cost erasure, cross-package sidecars, local fact injection via `pkg/trust`, relations via tuple subjects, and property-test helpers for inference rules.

Phase 9 (performance, caching, cross-process sharing patterned after [rewire](https://github.com/GiGurra/rewire)) is in progress. Compile-time evaluation of pure expressions — originally planned as `infer.Const` — was explored and declared out of scope (it's its own project). See [`docs/comptime.md`](docs/comptime.md) for the full exploration.

</details>

## Related work

- [`rewire`](https://github.com/GiGurra/rewire) — compile-time mocking via `-toolexec`. Shares the preprocessor shape this project adopted, and the cache-discipline findings.
- [`fl`](https://github.com/GiGurra/fl) — the thought experiment that motivated the constraint idea.

## Acknowledgements

100% vibe coded with [Claude Code](https://claude.ai). AST rewriting and compiler toolchains are well outside my comfort zone.

## License

MIT.
