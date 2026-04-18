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

Signatures stay plain Go — no wrappers, no generics ceremony, no struct decorations. Multiple predicates in a `That` / `Returns` call are AND-composed; for OR composition or first-class predicate values use `proven.All`, `proven.Any`, `proven.Not`.

Verify that a precondition is wired correctly in a test:

```go
proventest.AssertFails(t, isPositive, func() {
    Transfer(-5, "hi", "USD") // isPositive must reject -5
})
```

## How it works

**Preprocessor over `-toolexec`.** For each call to a function with `proven.That` / `proven.Returns` annotations, the preprocessor runs flow-sensitive analysis in the caller: facts from literals, preceding checks, early-return guards, and prior `proven.Returns` postconditions are collected; each predicate must be discharged. Proven obligations are erased from the compiled binary. Unproven ones fail the build.

**IDE-friendly link-time gate.** `proven.That` and `proven.Returns` wrap their check blocks in a package-private `atCompileTime` helper declared via `//go:linkname` to an external symbol (`_proven_atCompileTime`) with no Go body. `gopls`, `go vet`, and every editor see plain Go — type-checking is always green. But `go build` / `go test` on a `main` or test target refuses to link without the preprocessor, producing `relocation target _proven_atCompileTime not defined`. You cannot silently ship code that bypasses proven.

**Test-time verification.** `pkg/proventest` supplies the linker symbol for test binaries. By default it's a no-op (matches production). Inside `proventest.WithChecks(fn)` — or the higher-level `AssertFails(t, pred, fn)` — each `atCompileTime` block executes at runtime and a failing predicate panics with a structured `proven.Violation` naming the predicate that fired. Tests use this to assert "this parameter is constrained by this predicate", catching drift between assertion and implementation.

## Status

Pre-alpha. The runtime-stub API is in place (`pkg/proven`, `pkg/proventest`); the preprocessor itself is not yet built.

See [`docs/design.md`](docs/design.md) for the authoritative design, and [`docs/companion-packages.md`](docs/companion-packages.md) for the planned `prove` (runtime boundary validation) and `infer` (compile-time evaluation) siblings.

## Related work

- [`rewire`](https://github.com/GiGurra/rewire) — the `-toolexec` pipeline this project will reuse.
- [`fl`](https://github.com/GiGurra/fl) — the thought experiment that motivated the constraint / comptime ideas.

## License

MIT.
