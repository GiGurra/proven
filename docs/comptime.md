# Compile-time evaluation of pure expressions — out of scope

This document captures an exploration of "Zig-style comptime for Go" — evaluating pure expressions at build time, emitting their results as literals in the binary. It lived on the `proven` roadmap for a while under the name `infer.Const`, originally sourced from the project's [`concept.md`](concept.md). When we finally sat down to design it we concluded it should not be part of this project at all. This document preserves what we learned so the work is not lost if someone later wants to pick it up as its own project.

The reader does not need to read everything below. If you are just looking for the decision and why, the next two sections (**Why it does not belong here** and **The original motivation**) are enough. The rest is the full design space with notes from real code spikes, useful if you actually intend to build this elsewhere.

## Why it does not belong here

`proven` is a **compile-time contract system**. Its value proposition is "declare predicates on function arguments and return values, have the preprocessor discharge them at each call site, fail the build when any obligation remains unproven." Every phase that landed so far — stub injection, obligation scan, flow-sensitive discharge, inference rules, cross-package sidecars, runtime boundary validators, local fact injection, relations between values, runtime rule verification — serves that one value proposition directly.

Compile-time evaluation of pure expressions is a different thing entirely. The motivating use cases (lookup tables, configuration constants, code that would otherwise go in `go generate`) have nothing to do with contracts between caller and callee. The mechanisms are different: contracts are an AST-level static analysis, `Const` is build-time code execution. The hard problems are different: contracts need flow analysis and sidecar schemas, `Const` needs an execution model and a purity gate. The users who want one rarely need the other.

The original [`companion-packages.md`](companion-packages.md) argued for bundling by saying "both are what we deduce at build time." That is true but weak. Two features that both happen during the preprocessor pass do not automatically belong in the same package, and in this case the surface-area expansion does not earn its keep.

If and when someone wants to ship a comptime library for Go, it should be its own project. The proven preprocessor is reusable machinery for a library like that, but the API (`infer.Const` vs some other name in some other package) and the internal design (see below) should be built against real user code for comptime, not bolted onto a contract system.

## The original motivation

```go
var primes = infer.Const(sieve(10_000))
```

**Under the preprocessor:** `sieve(10_000)` runs during compilation; the resulting slice is emitted as a Go literal baked into the binary. Startup cost zero.

**Without the preprocessor:** `infer.Const` is the identity function; the expression evaluates at package-init time.

**Stated goals:** eliminate init-time cost for derived data, inline the result as a literal so the runtime sees a constant, surface purity violations as build errors.

## The design space we mapped

Two orthogonal axes dominate the design space:

- **Who evaluates the code:** roll our own (interpreter), run full Go (subprocess or embedded interpreter), or avoid evaluation entirely (symbolic or manifest-style).
- **When evaluation happens:** during toolexec, during a separate pre-pass the user triggers, or during tests.

Alternatives we enumerated:

| | Approach | One-line description |
|---|---|---|
| A | AST-walking interpreter over a Go subset | Roll your own. Full control over purity. Correctness burden scales with feature set. |
| B | Generated helper binary (`go run`) | Write a tiny `main` that imports user code, evaluates the expression, prints the result. Full Go semantics free. Purity must be gated externally. |
| C | Wait for Go comptime | Not on a useful timescale. |
| D | Yaegi (external Go interpreter) | Someone else maintains the interpreter. Not 100% Go-complete. Purity still external. |
| E | `go run` a temp file per expression | Lighter than B — no cached binary. Slower per-call. |
| F | `go/constant` only | Strict: literal arithmetic and string ops. No functions, no slices. Trivial to implement, covers ~5% of real use. |
| G | Hybrid: F fast path + fallback to A or B | Cover the trivial cases for free, pay only when you need richer evaluation. |
| H | SSA-based interpreter | Smaller instruction set than AST, but SSA is not a stable API and you still reimplement everything. |
| I | Constexpr — syntactic restriction + Go's constant machinery | User must write code in a dialect Go can already const-evaluate, plus pure-function inlining. Verifiable by grammar. |
| J | Manifest + verify | User writes the literal alongside the computation; test-time verifier checks equality. No build-time execution. |
| K | `go generate`-style explicit pre-pass | User runs `proven gen ./...` that evaluates Const expressions and writes a generated file. Not automatic. |
| L | Content-addressed cache + deferred compute | Preprocessor computes on first encounter (via B), caches by source hash. Rebuilds reuse. Amortizes B's cost. |

## Spikes we actually ran

We coded and executed the most interesting four approaches against a motivating sieve-of-Eratosthenes example to see what each really costs.

### F — `go/constant` alone

A small driver over `go/types.Check` plus `info.Types[expr].Value` evaluates constant expressions the Go type checker already handles:

```
expr: 2 + 3                    → type=int value=5
expr: 1 << 20                  → type=int value=1048576
expr: "hello" + " " + "world"  → type=string value="hello world"
expr: len("hello")             → type=int value=5
expr: 10 * (1 + 2)             → type=int value=30
expr: 1.0 / 3.0                → type=float64 value=0.333333
expr: []int{1, 2, 3}           → not a constant
expr: [3]int{1, 2, 3}          → not a constant
expr: map[string]int{"a": 1}   → not a constant
expr: func() int { return 42 }() → not a constant
```

**Verdict:** useful only as a fast path. Go's constant evaluator explicitly refuses composite literals, array literals, map literals, and function calls. Slices and maps — the overwhelming majority of realistic `Const` use cases — do not participate. This is the zero-work option, but the zero work reflects the zero coverage.

### B — generated helper binary

We wrote a tiny `runner/main.go` that imports `userpkg`, calls `userpkg.Sieve(100)`, and emits the result as JSON:

```
$ go run ./runner
[2,3,5,7,11,13,17,19,23,29,31,37,41,43,47,53,59,61,67,71,73,79,83,89,97]
```

This is what a preprocessor using approach B would do programmatically: parse user source, find each `infer.Const(expr)` call, generate a runner file that templates `expr` into a `fmt.Printf`/`json.Marshal`/`go/printer`-produced emitter, `go run` it, parse output, substitute the result back as a Go literal in a rewritten source file the compiler receives instead of the original.

**Worked cleanly. What makes it real work:**

- **Emitting the result as a Go literal** — JSON is easy but lossy (no `int32` vs `int`, no `uint`, custom struct types collapse to fields). A Go-source-literal printer via `go/printer` is more faithful but needs hand-rolling for every type shape the user might want to emit.
- **Purity gate** — the runner can do anything. I/O, syscalls, `time.Now()`, random numbers all run. Without static analysis of the call graph we have to trust the user, which means a bug in the user's "pure" function compiles to a broken binary.
- **Build-inside-build** — invoking `go run` from inside a toolexec pass launches another Go build. Needs careful `GOCACHE` discipline to avoid pathological recursion (the inner build's toolexec is the same proven binary, which is fine because the inner build's source does not contain `infer.Const` — but any misconfiguration here breaks builds mysteriously).
- **Per-expression cost** — one compile-and-run per Const call. Batching a whole package's Consts into one runner amortizes, but the first-time cost is meaningful.
- **Import resolution** — the runner imports the user's package, which needs the module's `go.mod` and transitive deps. Not hard, but a real plumbing step.

None of these are showstoppers. They just add up to "this is a whole new component in its own right, with its own set of gotchas."

### J — manifest + verify

The user writes the literal they want AND the computation that produces it. Runtime cost: zero — we just return the literal. At test time, a helper runs the computation and checks it matches.

```go
var Primes = Manifest(
    []int{2, 3, 5, 7, 11, /* ...full literal... */ 97},
    func() []int { return sieve(100) },
)

func TestPrimesManifest(t *testing.T) {
    infertest.VerifyManifest(t,
        []int{2, 3, 5, 7, 11, /* ... */ 97},
        func() []int { return sieve(100) })
}
```

**Worked trivially.** Runtime stub is five lines. No preprocessor involvement. The test catches any drift between the literal and the spec the next time the user runs `go test`.

Breaks down in one place: **large literals bloat source files.** A 10,000-prime slice is ~60 KB of source. Git diffs are painful.

But `//go:embed` handles this cleanly:

```go
//go:embed primes.json
var primesJSON []byte

var Primes = Manifest(decode[[]int](primesJSON), func() []int { return sieve(10_000) })
```

Now the "literal" lives in a separate file. The test's `-update` mode regenerates it when the spec changes. Source diffs are clean. Verification still works.

**The ergonomic bonus nobody asks for up front:** the user sees the exact value in their source or in an embedded data file. That is a real documentation artifact in ways `infer.Const` as originally proposed never would be.

### A — minimal AST interpreter

We wrote a ~200-line tree-walker supporting: int literals, string literals, unary + binary ops, if/else, for loops with init/cond/post, assignment, inc/dec, return, function calls, recursion. It correctly evaluates:

```
factorial(5)    = 120
factorial(10)   = 3628800
fib(20)         = 6765
2 + 3 * 4       = 14
```

What it does **not** support: slices, maps, strings beyond bare reads, index assignment (`s[i] = x`), `make`/`append` (we stubbed partial versions), composite literals (`[]int{1,2,3}`), struct literals and field access, type assertions, switch/case, break/continue, closures, function values, imports, generics, iterators.

To reach the sieve example specifically we would need slices, `make`, `append`, and index assignment — another ~200 lines at a minimum, each with its own edge cases. To reach realistic Const use cases (struct-valued tables, maps keyed by enum, trees) we would need struct literals and field access — several hundred more. To survive Go version upgrades we would need ongoing maintenance as the grammar evolves.

**Verdict:** 1,500+ lines of forever-maintenance interpreter code that reinvents what the Go toolchain already has. The AST interpreter is the option the original roadmap gravitated toward precisely because it is in-process and fast, but the long-tail feature chase is real and undisruptable.

## The ranking we landed on

1. **Manifest + verify (J), with `//go:embed` for large literals.** Zero build-time machinery. User sees the exact value in source or data file. Test-time `-update` workflow matches conventions Go programmers already know (golden tests, `go generate`). Breaks down only when the user genuinely cannot tolerate the manual-update step.
2. **J → B progressive.** Start with J. If the manual-update friction actually bites users, add a preprocessor step that rewrites the literal slot automatically from the spec's output. The preprocessor's job in that expansion is *refreshing an existing literal* from a pure spec — much more tractable than evaluating code into a vacuum because the user has already written one correct snapshot.
3. **Helper binary (B) alone.** Full automation, full Go semantics. Purity gate is the hard part; build-in-build orchestration is fiddly; per-Const compile cost amortized via a content-addressed cache (L).
4. Yaegi (D). External dep, still needs purity gate. Small wins for the size of the dependency.
5. AST interpreter (A). Do not. The scaling problem is too real and Go keeps adding features.
6. `go/constant` alone (F). Too narrow to matter standalone.

## The "execution model" question was a false choice

The original roadmap framed the decision as "interpret a subset vs run a helper binary vs wait for Go comptime." All three assume the preprocessor has to evaluate code at build time. The actual winning option is **do not evaluate at build time at all** — let the user hold the literal, verify it at test time, and refresh it on demand via a standard workflow. That option is missing from the roadmap because the framing narrowed it out. This is worth remembering for future design work on any phase: the framing of the question can smuggle in assumptions that eliminate the best answer.

## What would be needed to actually ship this elsewhere

If someone starts a separate comptime-for-Go library tomorrow, the work below is what they inherit:

- **Surface API.** `Manifest[T any](literal T, spec func() T) T` plus a test-time `VerifyManifest[T]` — plus whatever `-update` plumbing fits the library's ambitions.
- **Purity story.** For the J-only path, purity is entirely user-discipline — the test catches non-determinism. For any path that runs user code during the build, a purity gate (syntactic call-graph walk at minimum, static taint analysis at most) has to be designed and implemented.
- **Literal emission.** Go-source printing for arbitrary values that might appear on the left-hand side of a `Manifest` call. JSON is not enough; type identity and generated-code clarity matter.
- **Cache story.** Content-addressed by source hash keyed off the spec function's transitive dependencies. Go's build cache key does not include this, so the library has to roll its own or accept stale builds on cache hits.
- **Test tooling.** `-update` as a convention requires the library's tests to know how to find the right source file and rewrite the literal without disturbing surrounding code. `go/printer` produces reformatted output that can disturb unrelated lines; a token-level rewrite is cleaner.

None of this is out of reach. All of it is a separate project, not a phase of proven.
