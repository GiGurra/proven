package preprocessor

// compile-tool handling. The Go toolchain invokes `compile` once per
// package; toolexec wraps each call. We inspect the argv to learn
// which package is being compiled and which source files it owns,
// then dispatch to per-package handlers. Only one handler is live
// today — pkg/proven — but the structure leaves room for per-package
// obligation scanning in later phases without reshaping the call
// site in run.go.

// planCompile inspects a toolexec argv, decides whether a package
// needs augmentation, and returns the list of additional source
// files to append to the compile's argv plus a cleanup func. The
// extras are absolute paths to temp files; cleanup removes them
// after the caller forwards the compile. Non-compile invocations
// and packages we do not handle return (nil, nil, nil) — the caller
// forwards unchanged.
func planCompile(toolPath string, toolArgs []string) (extras []string, cleanup func(), err error) {
	if !isCompileTool(toolPath) {
		return nil, nil, nil
	}
	pkgPath := compilePkgPath(toolArgs)
	if pkgPath == "" {
		return nil, nil, nil
	}

	switch pkgPath {
	case provenPkgPath:
		files := compileSourceFiles(toolArgs)
		return planProvenStub(files)
	}
	return nil, nil, nil
}

// compilePkgPath returns the value of the -p flag in a compile argv,
// or "" if not present. The Go toolchain always emits -p <importpath>
// as two tokens.
func compilePkgPath(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-p" {
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
