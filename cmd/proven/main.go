// Command proven is the proven preprocessor — a -toolexec binary
// that hooks into the Go build pipeline.
//
// Usage:
//
//	go build -toolexec=/path/to/proven ./...
//
// In toolexec mode, os.Args[1] is the absolute path of a Go tool
// (compile, link, asm, ...) and os.Args[2:] are the arguments for
// that tool. All behavior lives in internal/preprocessor; this
// binary is a thin shim around its Run function so the preprocessor
// itself is unit-testable without spawning a subprocess.
package main

import (
	"os"

	"github.com/GiGurra/proven/internal/preprocessor"
)

func main() {
	os.Exit(preprocessor.Run(os.Args[1:], os.Stderr))
}
