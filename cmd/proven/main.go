// Command proven is the proven preprocessor — a -toolexec binary that
// hooks into the Go build pipeline.
//
// For now this is a stub that forwards every Go-tool invocation to the
// real underlying tool unchanged. Subsequent iterations will add the
// per-package scan of proven.That / proven.Returns / infer.From rules,
// flow-sensitive discharge analysis, AST rewriting (erase discharged
// calls, supply the _proven_atCompileTime linker symbol), and
// diagnostic emission.
//
// Usage as a toolexec wrapper:
//
//	go build -toolexec=/path/to/proven ./...
//
// In toolexec mode, os.Args[1] is the absolute path of a Go tool
// (compile, link, asm, ...) and os.Args[2:] are the args for that
// tool.
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "proven: expected toolexec invocation (<tool-path> [args...])")
		os.Exit(2)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "proven: failed to run %s: %v\n", args[0], err)
		os.Exit(1)
	}
}
