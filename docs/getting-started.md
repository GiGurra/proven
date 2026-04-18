# Getting Started

## Install

```bash
# Install the preprocessor binary
go install github.com/GiGurra/proven/cmd/proven@latest

# Add the runtime packages to your module
go get github.com/GiGurra/proven
```

The preprocessor binary is the `-toolexec` program the Go toolchain invokes for every compile action in your build. It lives outside your module's dependency graph — it's a build-time tool, not a library.

The runtime packages (`pkg/proven`, `pkg/prove`, `pkg/trust`, `pkg/infer`, `pkg/infertest`, `pkg/proventest`) live inside the `github.com/GiGurra/proven` module. `go get` pulls them in so you can import them in your source.

## First passing build

Create a tiny program and build it under the preprocessor:

```go
// main.go
package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func double(x int) int {
	proven.That(x, isPositive)
	return x * 2
}

func main() {
	x := 5
	if isPositive(x) {
		fmt.Println(double(x)) // isPositive(x) is proven; compiler accepts the call
	}
}
```

Build with the preprocessor active:

```bash
GOFLAGS="-toolexec=proven" go build ./...
```

Nothing should print — the build succeeds.

### What the guard actually does

The line `if isPositive(x)` is not a special "proof" API. It's an ordinary Go function call in an ordinary `if`. The preprocessor sees it, notes that `isPositive(x)` has been evaluated to true on the then-branch, and treats it as a **fact**. When you call `double(x)` inside that branch, the preprocessor looks at `double`'s declared precondition (`proven.That(x, isPositive)`) and sees that the same fact is in scope — the precondition is proven and the call is accepted.

No separate proof object. No generics ceremony. Just Go control flow.

You can produce the same fact in other ways — `if !isPositive(x) { return }` then a call on the following line, `v, err := prove.That(raw, isPositive)` then an `if err != nil { return }` guard, a function whose body proves `isPositive` on its returned identifier (with or without an explicit `proven.Returns`), or a `trust.That(raw, isPositive)` injection. All of them produce the same `{predicate: isPositive, var: x}` fact in the analyzer, and all of them prove `double(x)`'s precondition identically. The right choice is whichever matches the shape of the surrounding code.

### What a missing proof looks like

Remove the guard:

```go
// main.go
package main

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool { return x > 0 }

func double(x int) int {
	proven.That(x, isPositive)
	return x * 2
}

func main() {
	x := readFromSomewhere()
	fmt.Println(double(x)) // no guard — build fails
}

func readFromSomewhere() int { return 5 }
```

Rebuild and the preprocessor refuses:

```
main.go:NN:CC: proven: cannot prove isPositive on parameter 0 of double
```

Same `file:line:col:` format Go's own compiler uses, so your editor click-through works the same as any build error. The diagnostic names the missing predicate, the parameter index, and the callee — so the fix is one of:

- Add an explicit guard: `if isPositive(x) { fmt.Println(double(x)) }`
- Normalize the value at a boundary: `v, err := prove.That(x, isPositive); if err != nil { ... }; fmt.Println(double(v))`
- Let the producer advertise the fact. If `readFromSomewhere`'s body establishes `isPositive` on its returned identifier — via a guard, a `prove.Must`, or its own declared precondition — callers pick the postcondition up automatically. Wrap the return in `proven.Returns(v, isPositive)` to make the contract a compiler-verified declaration at the site.
- Take responsibility: `v := trust.That(x, isPositive); fmt.Println(double(v))` — if `x` was validated by a mechanism the analyzer cannot see.

Each produces the same fact; the preprocessor accepts the build after any of them.

### How far does the proof carry?

As far as you pass the value, as long as each function along the way declares what it expects.

```go
func target(x int) {
    proven.That(x, isPositive)  // target's own precondition
    deeper(x)                   // deeper requires isPositive — already proven,
                                // because target's own precondition says so.
}

func deeper(x int) {
    proven.That(x, isPositive)
    // ...
}
```

Inside `target`'s body, `x` is known to be `isPositive` — that's the analyzer's starting fact set when analyzing `target`. So `target` can call `deeper(x)` without re-proving. The caller of `target` had to prove `target`'s precondition once, at the call site. From there the fact rides along through every function that declares the same precondition and passes the value through.

You don't re-prove at every hop. You declare what each function needs once, and the caller proves it once at its call site.

## Cache discipline

Go's build cache key does **not** include toolexec state, so a cached `pkg/proven.a` from a plain `go build` (no preprocessor) and one from a `-toolexec=proven` build (with the preprocessor injecting the linker stub) collide. Symptoms:

- `relocation target _proven_atCompileTime not defined` — toolexec build reused a stub-less artifact.
- `duplicated definition of symbol _proven_atCompileTime` — non-toolexec test binary (pulling in `proventest`) reused a stub-containing artifact.

The safest setup is a **dedicated GOCACHE for toolexec builds**.

### Terminal

```bash
alias gobuild-proven='GOFLAGS="-toolexec=proven" GOCACHE="$HOME/.cache/proven-build" go build'
alias gotest-proven='GOFLAGS="-toolexec=proven" GOCACHE="$HOME/.cache/proven-build" go test'
```

### GoLand

Run → Edit Configurations → Templates → Go Test → Environment variables:

```
GOFLAGS=-toolexec=proven
GOCACHE=/Users/<you>/.cache/proven-build
```

Same for Go Build templates if you want `go build` inside the IDE to go through the preprocessor.

### VS Code (settings.json)

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

### Cleaning

`go clean -cache` wipes whichever cache `$GOCACHE` currently points at. To clean the proven cache specifically:

```bash
GOCACHE="$HOME/.cache/proven-build" go clean -cache
```

## Writing tests

Import `pkg/proventest` in your test files — it supplies the linker symbol the preprocessor would otherwise provide, so test binaries link cleanly:

```go
import "github.com/GiGurra/proven/pkg/proventest"
```

The default state is **no-op**: `proven.That` blocks do not execute at runtime, matching production behavior. To opt into runtime verification inside a test, wrap with `WithChecks` or use one of the higher-level helpers.

### Testing that bad input is rejected

`AssertFails(t, pred, fn)` asserts "running `fn` under `WithChecks` causes the named predicate to fire":

```go
func TestTransfer_RejectsNegativeAmount(t *testing.T) {
    proventest.AssertFails(t, isPositive, func() {
        Transfer(-5, "hi") // isPositive must reject -5
    })
}
```

If someone later drops `proven.That(amount, isPositive)` from `Transfer`, this test fails — no predicate fired when one was expected. If the predicate is weakened to `isNonNegative`, the test fails with `expected isPositive to fire, got isNonNegative`.

### Testing that good input is accepted

`AssertPasses(t, fn)` is the symmetric counterpart: "running `fn` under `WithChecks` produces no violation, i.e. every declared precondition and postcondition accepts the input":

```go
func TestTransfer_AcceptsValidInput(t *testing.T) {
    proventest.AssertPasses(t, func() {
        Transfer(5, "hi") // every contract along the call chain must accept
    })
}
```

If someone later tightens `isPositive` into, say, `isGreaterThanTen`, this test fails with `expected no violation, but isGreaterThanTen fired on value=5`. Use it to lock in known-valid example inputs — so a refactor that accidentally over-narrows a predicate is caught immediately.

### Lower-level primitives

- `WithChecks(fn)` — raw: runs `fn` with check blocks enabled; propagates any `proven.Violation` panic. Use it when you want to compose something the Assert helpers don't cover.
- `AssertAnyFailure(t, fn)` — weaker than `AssertFails`: asserts that *some* proven violation fires, returns the `proven.Violation` for further inspection. Use when the exact predicate identity doesn't matter (e.g. you only care that the function rejected the input, not which declared predicate did the rejecting).

## Where to go next

- **[Design](design.md)** — the authoritative design: the call shape, the link-time gate, the rationale over earlier alternatives.
- **[Companion Packages](companion-packages.md)** — how each package (`proven`, `prove`, `trust`, `infer`, `infertest`) fits together.
- **[Relations Between Values](relations.md)** — the tuple-subject pattern for multi-value properties.
