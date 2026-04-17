# proven

Compile-time proven constraints for Go, via `-toolexec` preprocessing.

## What is this?

`proven` lets you attach refinement constraints to Go values (`Positive`, `NonEmpty`, `ValidEmail`, custom predicates) and have them checked **statically** at build time, not at runtime. When the checker can prove a value satisfies its constraint along every path that reaches a consumer, no runtime code is emitted. When it can't, the build fails — or, at designated boundaries (HTTP handlers, deserialization sites), a runtime guard is injected.

The goal is simple: **stop forgetting to validate incoming data**, and stop repeating the same validation at every layer once you have.

## Status

Early-stage. Concept doc and toolchain scaffolding only — no working checker yet.

See [`docs/concept.md`](docs/concept.md) for the design.

## Related work

- [`rewire`](https://github.com/GiGurra/rewire) — the `-toolexec` pipeline this project reuses.
- [`fl`](https://github.com/GiGurra/fl) — the thought experiment that motivated the refinement-type ideas.

## License

MIT.
