# proven

Compile-time contracts for Go, enforced via a `-toolexec` preprocessor.

Systems grow organically. Past a certain size it becomes effectively impossible to know what state a program is in when it reaches any given point in its call graph — or even remember why a particular requirement was imposed in the first place. Engineers re-validate defensively, trust invariants that may have drifted, or simply forget. The cost is runtime bugs, redundant validation at every layer, and a maintenance burden that scales faster than the codebase.

`proven` shifts that burden onto the compiler. A function declares what must hold about its inputs or its program state; the preprocessor walks the entire call graph and proves that every path complies. If the proof succeeds, nothing runs at runtime. If any path can't be proved, the build fails with a diagnostic pointing at the offending call site.

The point isn't to add more validation. It's to stop having to remember.

Declare preconditions and postconditions inside function bodies using plain `func(T) bool` predicates:

```go
func isPositive(x int) bool    { return x > 0 }
func isNonEmpty(s string) bool { return len(s) > 0 }
func maxLen280(s string) bool  { return len(s) <= 280 }

func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty, maxLen280)
    // ... body ...
    return nil
}

func FindUserID() int {
    return proven.Returns(42, isPositive)
}
```

Signatures stay plain Go — no wrappers, no generics ceremony, no struct decorations. Multiple predicates in a single `That` / `Returns` call are AND-composed, so the common case requires nothing extra. For OR composition, negation, or a reusable composite predicate you can pass around, use `proven.And`, `proven.Or`, `proven.Not`:

```go
var validCurrency = proven.Or(isUSD, isEUR, isGBP)
var sensibleQty   = proven.And(isPositive, lessThan1000)
var eligibleUser  = proven.Not(isBanned)

func Charge(amount int, currency string, userID int) error {
    proven.That(amount,   sensibleQty)
    proven.That(currency, validCurrency)
    proven.That(userID,   eligibleUser)
    // ...
}
```

The combinators return plain `func(T) bool`, so they compose freely and can be stored in package-level variables, looked up from maps, passed to `Returns`, or supplied directly to `That`.

When you want a call site that has established one predicate (say `isSmallPositive`) to satisfy a callee that requires a different but related predicate (say `isPositive`) without re-proving, declare the inference rule at package scope:

```go
// infer: isSmallPositive ⇒ isPositive
var _ = infer.From(isSmallPositive).To(isPositive)

// infer: isEven ⇒ isPositive, when the value is > 0
var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
```

The preprocessor consumes these at scan time and walks the resulting implication graph when discharging obligations. Rules are trusted — the preprocessor does not symbolically verify them. The declarer is responsible, and a future `infertest.Verify` helper will property-test rules on sample inputs.

Compile-time contracts aren't new — Eiffel's Design by Contract, Ada's `Pre`/`Post` clauses, C++ `static_assert` and concepts, and Rust's type-state patterns are all takes on the same idea. Their shared failure mode is that a requirement verified only at compile time is fragile: nothing catches you accidentally removing it (*"why is this line here?"*), weakening it (someone narrows `isPositive` to `isNonNegative` in a refactor), or forgetting to state it in the first place.

`proven` addresses this by letting you verify at *test* time that the declarations are what you think. The preprocessor erases `proven.That` at build as usual, but inside tests you can opt in to running the blocks at runtime and asserting that the right predicate fires on the right parameter:

```go
proventest.AssertFails(t, isPositive, func() {
    Transfer(-5, "hi", "USD") // isPositive must reject -5
})
```

If someone later drops `proven.That(amount, isPositive)` from `Transfer`, this test fails — no violation fires. If they replace `isPositive` with a weaker predicate, the test fails with `expected isPositive to fire, got isNonNegative`. Production still runs with zero overhead; the runtime mode is strictly additive, and the test suite now defends the contract from silent drift.

## How it works

**Preprocessor over `-toolexec`.** For each call to a function with `proven.That` / `proven.Returns` annotations, the preprocessor runs flow-sensitive analysis in the caller: facts from literals, preceding checks, early-return guards, and prior `proven.Returns` postconditions are collected; each predicate must be discharged. Proven obligations are erased from the compiled binary. Unproven ones fail the build.

**IDE-friendly link-time gate.** `proven.That` and `proven.Returns` wrap their check blocks in a package-private `atCompileTime` helper declared via `//go:linkname` to an external symbol (`_proven_atCompileTime`) with no Go body. `gopls`, `go vet`, and every editor see plain Go — type-checking is always green. But `go build` / `go test` on a `main` or test target refuses to link without the preprocessor, producing `relocation target _proven_atCompileTime not defined`. You cannot silently ship code that bypasses proven.

**Test-time verification.** `pkg/proventest` supplies the linker symbol for test binaries. By default it's a no-op (matches production). Inside `proventest.WithChecks(fn)` — or the higher-level `AssertFails(t, pred, fn)` — each `atCompileTime` block executes at runtime and a failing predicate panics with a structured `proven.Violation` naming the predicate that fired. Tests use this to assert "this parameter is constrained by this predicate", catching drift between assertion and implementation.

## Status

Pre-alpha. Runtime API (`pkg/proven`, `pkg/proventest`, `pkg/prove`, `pkg/infer`) is in place. The preprocessor is under construction: Phase 1 — stub injection so any program using `proven.That` links under `-toolexec=proven` — is done. Flow-sensitive discharge, rewrite-on-success, diagnostic-on-failure, and cross-package obligation summaries follow.

See [`docs/design.md`](docs/design.md) for the authoritative design, [`docs/companion-packages.md`](docs/companion-packages.md) for the planned `prove` (runtime boundary validation) and `infer` (compile-time evaluation) siblings, and [`docs/todo/roadmap.md`](docs/todo/roadmap.md) for the preprocessor plan.

## Related work

- [`rewire`](https://github.com/GiGurra/rewire) — the `-toolexec` pipeline this project reuses: AST-based scanning of compile argv, temp-file source augmentation, and cache-invalidation strategy.
- [`fl`](https://github.com/GiGurra/fl) — the thought experiment that motivated the constraint / comptime ideas.

## License

MIT.
