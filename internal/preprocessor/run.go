// Package preprocessor implements the proven -toolexec preprocessor.
//
// Run is the single entrypoint. It is called from cmd/proven's main
// with os.Args[1:] and returns a shell-style exit code. Everything
// below is intentionally private: the preprocessor is distributed as
// the proven binary, not as an importable library.
//
// Shape follows rewire (github.com/GiGurra/rewire) closely: a fast
// dispatch on the tool being wrapped, AST-based scanning of the
// source files the Go tool received, a minimal set of rewrites
// emitted to temp files that are appended to the tool's argv, and a
// final forward to the real tool. Nothing in the original source
// tree is modified.
//
// See docs/todo/roadmap.md for the sequence of phases; the current
// implementation covers Phase 1 (stub injection for pkg/proven) only.
package preprocessor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes the preprocessor with the toolexec-style arguments
// (os.Args[1:]): the first element is the path to the underlying Go
// tool (compile, link, asm, ...), and the rest are the tool's own
// arguments. Run returns the exit code to hand back to the OS.
//
// Behavior is opaque to the caller: any diagnostics go to the
// supplied stderr, any synthesized sources live in temp dirs and are
// cleaned up deferred.
func Run(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "proven: expected toolexec invocation (<tool-path> [args...])")
		return 2
	}

	toolPath, toolArgs := args[0], args[1:]
	extraSources, cleanup, err := planCompile(toolPath, toolArgs)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		fmt.Fprintf(stderr, "proven: %v\n", err)
		return 1
	}
	toolArgs = append(toolArgs, extraSources...)

	cmd := exec.Command(toolPath, toolArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "proven: failed to run %s: %v\n", toolPath, err)
		return 1
	}
	return 0
}

// isCompileTool reports whether the tool being invoked is the Go
// compiler. The Go toolchain binaries live under $GOTOOLDIR; only the
// basename (with an optional .exe suffix on Windows) identifies the
// tool.
func isCompileTool(toolPath string) bool {
	base := strings.TrimSuffix(filepath.Base(toolPath), ".exe")
	return base == "compile"
}
