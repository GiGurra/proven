# Go-language findings

Empirical facts about Go 1.26 that the proven design relies on. If a future Go release changes any of these, revisit the parts of the design that depend on them. These were originally captured in `internal/*experiment/` packages during design exploration; those packages were deleted once the design settled, and this file replaces them as the persistent record.

## Phantom type parameters are not inferable from call-site context

Go 1.26 cannot infer a generic function's type parameter if the parameter does not appear in the argument list, even when the return-type context would unambiguously pin it:

```go
func f[P any, T any](x T) Refined[P, T] { ... }

var r Refined[Positive, int] = f(42)  // FAILS: "cannot infer P"
useRefined(f(42))                     // FAILS: "cannot infer P"
```

This extends to return-type-only phantom parameters:

```go
func g[A any, B any](a A) B { ... }

useInt(g(42))  // FAILS: "cannot infer B", even when B is unambiguously int
```

**Consequence.** `proven.That` / `proven.Returns` do not take phantom type parameters. Predicates are values of type `func(T) bool`, passed as arguments — not type parameters. The earlier `Refined[P, T]` design was abandoned because users would have had to write `proven.Attest[Positive](x)` at every call site.

## Bare type parameters cannot be the RHS of generic aliases or named types

Go 1.26 rejects both of these forms:

```go
type Where[T any, P any] = T   // REJECTED: "cannot use type parameter declared in alias declaration as RHS"
type Where[T any, P any] T     // REJECTED: "cannot use a type parameter as RHS in type declaration"
```

Only non-generic per-combination aliases work:

```go
type PositiveInt = int         // ACCEPTED
```

**Consequence.** The "invisible generic wrapper" idea — one alias type that resolves to its first type argument while carrying phantom metadata in its second — is not expressible in Go. This closed off one of the design directions we explored.

## `//go:linkname` defers resolution to the linker

A function declaration marked with `//go:linkname local external` and *no Go body* type-checks, compiles, and defers symbol resolution to the link step:

```go
//go:linkname atCompileTime _proven_atCompileTime
func atCompileTime(_ func())
```

If no package in the link graph supplies `_proven_atCompileTime`, the linker fails with:

```
<package>.<caller>: relocation target _proven_atCompileTime not defined
```

**Consequence.** This is the mechanism of proven's link-time gate. `pkg/proven/proven.go` declares `atCompileTime` via `//go:linkname`; `pkg/proventest` supplies the symbol for test binaries; the preprocessor will supply it for production builds. `gopls` and `go vet` see ordinary Go (type-checking is oblivious to link-time resolution), so the IDE experience stays green. See `docs/design.md` for the full rationale.

## When to revisit these findings

These are specifically Go 1.26 facts. Revisit when:

- A new Go release ships with changes to generic type inference.
- The type-parameter-as-RHS restriction is relaxed (watch Go release notes).
- `//go:linkname` semantics are tightened (Go has been making this more restrictive — Go 1.23 already required all `linkname`d packages to opt in in some cases).

A small regression probe in `cmd/proven` or `pkg/proven` that intentionally exercises each behavior would catch changes automatically; for now, the live code (the `atCompileTime` declaration, the `proven.That` signature shape) is the implicit regression suite, but any failure would only show up in the preprocessor-implementation phase, not in CI today.
