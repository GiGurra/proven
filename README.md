# proven

Compile-time contracts for Go, with runtime fallback. A `-toolexec` preprocessor proves preconditions at build time; when it can't, they degrade to runtime checks.

## What is this?

Declare a function's preconditions inside its body using `proven.That`:

```go
func Transfer(amount int, note string) error {
    proven.That(amount, isPositive)
    proven.That(note, isNonEmpty, maxLen280)
    // ... body ...
    return nil
}
```

The function signature stays pure Go — no wrapper types, no type parameters, no struct decorations. `gopls` and every editor see ordinary Go.

**IDE experience is always green.** `gopls`, `go vet`, and every Go-aware editor see ordinary Go — no red squiggles, no special configuration, no build tags.

**Link fails without the preprocessor.** `proven.That` and `proven.Returns` wrap their checks in an `atCompileTime` helper declared via `//go:linkname` to an external symbol the preprocessor supplies. Any `go build` / `go test` of a main or test target refuses to link without the preprocessor, with a clear error: `relocation target _proven_atCompileTime not defined`. You cannot silently ship code that bypasses proven.

**Under the preprocessor**, call sites are discharged by flow-sensitive analysis (literals, preceding checks, early-return guards, postconditions on return values). Discharged calls are erased; undischarged calls fail the build with a diagnostic.

The goal: **stop forgetting to validate incoming data**, and stop repeating the same validation at every layer once you have.

## Status

Pre-alpha. The runtime-stubs API is in place (`pkg/proven`). The preprocessor itself is not yet built.

See [`docs/design.md`](docs/design.md) for the authoritative design, and [`docs/companion-packages.md`](docs/companion-packages.md) for the planned `prove` (runtime boundary validation) and `infer` (compile-time evaluation) siblings.

## Related work

- [`rewire`](https://github.com/GiGurra/rewire) — the `-toolexec` pipeline this project will reuse.
- [`fl`](https://github.com/GiGurra/fl) — the thought experiment that motivated the constraint / comptime ideas.

## License

MIT.
