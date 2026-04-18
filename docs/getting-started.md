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
		fmt.Println(double(x)) // discharged
	}
}
```

Build with the preprocessor active:

```bash
GOFLAGS="-toolexec=proven" go build ./...
```

Nothing should print — the build succeeds. If you remove the `if isPositive(x)` guard, the build fails with:

```
main.go:NN:CC: proven: undischarged predicate isPositive on parameter 0 of double
```

Exactly the format `gopls` and editors already know how to click through.

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

The default state is **no-op**: `proven.That` blocks do not execute at runtime, matching production behavior. To opt into runtime verification inside a test, wrap with `WithChecks` or use the higher-level helpers:

```go
func TestTransfer_RejectsNegativeAmount(t *testing.T) {
    proventest.AssertFails(t, isPositive, func() {
        Transfer(-5, "hi") // isPositive must reject -5
    })
}
```

If someone later drops `proven.That(amount, isPositive)` from `Transfer`, this test fails with a message naming the predicate that was expected but did not fire.

## Where to go next

- **[Design](design.md)** — the authoritative design: the call shape, the link-time gate, the rationale over earlier alternatives.
- **[Companion Packages](companion-packages.md)** — how each package (`proven`, `prove`, `trust`, `infer`, `infertest`) fits together.
- **[Relations Between Values](relations.md)** — the tuple-subject pattern for multi-value properties.
