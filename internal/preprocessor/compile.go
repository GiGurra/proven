package preprocessor

// compile-tool handling. The Go toolchain invokes `compile` once
// per package; toolexec wraps each call. We inspect the argv to
// learn which package is being compiled and which source files it
// owns, then dispatch to per-package handlers:
//
//   - pkg/proven gets the Phase 1 linker-stub companion file
//     appended to the compile argv.
//   - every other package gets the Phase 5 scan+analyze pass; if
//     any call-site obligation is undischarged the preprocessor
//     emits Go-standard file:line:col: diagnostics and exits
//     non-zero, otherwise the compile forwards unchanged.
//
// All planning work returns a Plan that the caller (run.go) reads
// to decide which tool arguments to forward, which temp files to
// clean up, and whether to abort the build with diagnostics.

import (
	"fmt"
)

// Plan is the decision made for one toolexec compile invocation.
//
// NewArgs is the argv to forward to the real compile tool, or nil
// to leave the incoming toolArgs unchanged.
//
// Cleanup is deferred after the forwarded compile returns; may be
// nil.
//
// Diags, if non-empty, is printed to stderr by the caller and the
// preprocessor exits non-zero without forwarding the compile. A
// Plan may carry both NewArgs and Diags only if a handler decides
// to emit diagnostics after partially rewriting — not a case the
// current implementation exercises.
type Plan struct {
	NewArgs []string
	Cleanup func()
	Diags   []Diagnostic
}

// Diagnostic is one Go-standard preprocessor message. The String
// form matches `cmd/compile`'s `file:line:col: message` layout so
// editors and CI output parsers pick it up without special casing.
type Diagnostic struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: %s", d.File, d.Line, d.Col, d.Msg)
}

// planCompile inspects a toolexec argv and returns the Plan for
// it. A nil Plan means "forward toolArgs unchanged". A non-nil
// Plan replaces them (via NewArgs) and/or aborts the build (via
// Diags). An error return is reserved for preprocessor
// infrastructure failures (temp-file I/O, parse errors on the
// compiler's own source inputs).
func planCompile(toolPath string, toolArgs []string) (*Plan, error) {
	if !isCompileTool(toolPath) {
		return nil, nil
	}
	pkgPath := compilePkgPath(toolArgs)
	if pkgPath == "" {
		return nil, nil
	}

	switch pkgPath {
	case provenPkgPath:
		return planProvenStub(toolArgs)
	}

	return planUserPackage(pkgPath, toolArgs)
}

// compilePkgPath returns the value of the -p flag in a compile argv,
// or "" if not present. The Go toolchain always emits -p <importpath>
// as two tokens.
func compilePkgPath(args []string) string {
	return compileFlagValue(args, "-p")
}

// compileOutputPath returns the value of the -o flag in a compile
// argv, or "" if not present. The Go toolchain emits -o <path> as
// two tokens and the compile tool refuses to run without it.
func compileOutputPath(args []string) string {
	return compileFlagValue(args, "-o")
}

// compileImportcfg returns the value of the -importcfg flag in a
// compile argv, or "" if not present (unusual but legal for a
// package that imports nothing beyond unsafe and builtin).
func compileImportcfg(args []string) string {
	return compileFlagValue(args, "-importcfg")
}

// compileFlagValue is the shared helper behind the per-flag
// extractors. The compile tool uses the two-token form
// `-flag value` exclusively (not `-flag=value`); the helper
// relies on that invariant.
func compileFlagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// compileSourceFiles returns the .go source files appearing as
// trailing positional args of a compile invocation. Flags precede
// positional args; no Go-toolchain flag takes a .go-suffixed value,
// so a simple suffix filter over the argv is sufficient in practice
// and avoids a full flag schema.
func compileSourceFiles(args []string) []string {
	out := make([]string, 0, 4)
	for _, a := range args {
		if len(a) > 3 && a[len(a)-3:] == ".go" {
			out = append(out, a)
		}
	}
	return out
}
