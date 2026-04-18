package preprocessor

// Phase 5: per-user-package scan + analyze + diagnose + rewrite.
//
// For every compile invocation whose target is not pkg/proven, we
// parse the source files the compiler received, build the
// obligation summary (Phase 2), and analyze each caller's body
// for undischarged obligations (Phases 3–4). When any call-site
// obligation remains unproven, the preprocessor emits
// Go-standard diagnostics and the build fails without reaching
// the real compile tool.
//
// When every obligation is discharged, Phase 5b erases the
// discharged proven.That / proven.Returns calls from the source
// bytes the compiler will see: each original .go path is replaced
// in the compile argv with a temp copy whose rewrite is
// length-preserving, so cmd/compile's position information still
// points at the user's original columns. Programs compiled under
// the preprocessor therefore have no residual closure allocation
// or linkname-resolved call at sites the analyzer cleared.
//
// Packages with no obligations skip both analysis and rewriting —
// the scanner short-circuits on the import check, and the rewrite
// pass is only entered for files that import pkg/proven.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

// planUserPackage handles the compile of any package other than
// pkg/proven. The function returns one of:
//
//   - nil Plan, nil error: the original compile argv forwards
//     unchanged (no obligations in this package, or no files
//     needed rewriting).
//
//   - non-nil Plan with NewArgs set: one or more source files in
//     the argv were replaced by rewritten temp copies. Cleanup
//     deletes the temp files after the compile returns.
//
//   - non-nil Plan with Diags set: a discharge gap was found; the
//     build aborts before the compile runs, with one diagnostic
//     per missing predicate per call site.
//
//   - non-nil error: an infrastructure failure (a source file
//     failed to parse, or a temp write failed). The Go toolchain
//     would surface the parse error itself if forwarded, so we
//     report ours as "proven: ...".
//
// Packages with no .go source files in their argv (unusual but
// legal — cgo stubs, auto-generated stdlib artifacts) short-
// circuit immediately.
func planUserPackage(pkgPath string, toolArgs []string) (*Plan, error) {
	sources := compileSourceFiles(toolArgs)
	if len(sources) == 0 {
		return nil, nil
	}

	fset, files, err := parsePackageSources(sources)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", pkgPath, err)
	}

	sum := &PackageSummary{
		ImportPath: pkgPath,
		Funcs:      make(map[string]*FuncSummary),
	}
	for _, f := range files {
		scanFile(sum, pkgPath, f)
	}

	// Imported-package summaries. Sidecars written during earlier
	// per-dependency compiles live next to the .a files the Go
	// toolchain hands us via -importcfg. Missing sidecars are
	// silently absent — a package we never scanned (stdlib,
	// third-party without the preprocessor, or a package with no
	// obligations that skipped writing) simply contributes nothing
	// to cross-package discharge. A read error for a present file
	// is treated as non-fatal for the same reason: the compile
	// itself will still run; at worst the caller sees an
	// undischarged-obligation diagnostic the summary would have
	// cleared.
	imports, _ := readImportSummaries(compileImportcfg(toolArgs))

	// No obligations and no imported summaries contribute anything
	// actionable. The package uses neither proven.That nor
	// proven.Returns in a way the scanner cares about, and no
	// imported callee's obligations apply here either (the
	// analyzer records a discharge only when the summary has
	// ParamPreds, which comes from the callee itself). Nothing
	// to check, nothing to erase, no sidecar to write.
	//
	// sum.Rules carries declared inference rules (`var _ =
	// infer.From(p).To(q)`), which are part of the downstream
	// discharge story even for packages with no local obligations:
	// a rules-only package must still write its sidecar so callers
	// that import it pick up the implications via the Phase 6
	// sidecar-lookup path.
	if len(sum.Funcs) == 0 && len(sum.Rules) == 0 && len(imports) == 0 {
		return nil, nil
	}

	// Run discharge analysis. Diagnostics take precedence over
	// rewriting and sidecar emission — a build that is about to
	// fail never gets its sources rewritten, and its summary must
	// not be written because downstream packages would then
	// discharge against a function whose own callers are still
	// unproven.
	discharges := analyzePackageFiles(sum, files, imports)
	var diags []Diagnostic
	for _, d := range discharges {
		if !d.Undischarged() {
			continue
		}
		diags = append(diags, dischargeDiagnostics(d, fset, pkgPath)...)
	}
	if len(diags) > 0 {
		return &Plan{Diags: diags}, nil
	}

	// Emit this package's sidecar so packages that import it (in
	// the same `go build` invocation) can discharge cross-package
	// obligations against it. Write is best-effort: a failure to
	// write does not abort the compile, since downstream will
	// simply miss the obligation information and over-report
	// undischarged-ness, which is a soft failure mode we can
	// surface through later diagnostics rather than this one.
	if len(sum.Funcs) > 0 || len(sum.Rules) > 0 {
		_, _ = writeSummarySidecar(compileOutputPath(toolArgs), sum)
	}

	// Packages with obligations only from imports (no local
	// scanned Funcs) have nothing to rewrite — rewriting only
	// touches proven.That / proven.Returns calls in the current
	// package's own source.
	if len(sum.Funcs) == 0 {
		return nil, nil
	}

	// All obligations discharged. Rewrite every file that actually
	// has proven.That / proven.Returns calls to erase them. Files
	// without rewrite targets keep their original paths in the
	// compile argv.
	return rewritePlan(toolArgs, sources, files, fset)
}

// parsePackageSources parses every .go file once. The returned
// files align with sources by index. The shared fset owns the
// Pos values inside each file so the analyzer and rewriter can
// translate positions to (file, line, col).
func parsePackageSources(sources []string) (*token.FileSet, []*ast.File, error) {
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(sources))
	for _, src := range sources {
		f, err := parser.ParseFile(fset, src, nil, 0)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}
	return fset, files, nil
}

// analyzePackageFiles runs the flow-sensitive analyzer over every
// FuncDecl in every file, against the shared package summary.
// Factored out so planUserPackage can interleave rewriting after
// discharge without re-parsing. imports maps each imported
// package's import path to its PackageSummary (read from its
// sidecar during the compile of the current package) so
// cross-package obligations participate in discharge.
func analyzePackageFiles(sum *PackageSummary, files []*ast.File, imports map[string]*PackageSummary) []CallDischarge {
	var all []CallDischarge
	for _, f := range files {
		imp := collectImports(f)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				all = append(all, AnalyzeFunc(fn, sum, imp, imports)...)
			}
		}
	}
	return all
}

// rewritePlan walks each source file, rewrites any proven.That /
// proven.Returns calls to blanks (preserving columns), and
// produces a new toolArgs argv in which each rewritten path is
// substituted for its original. Files that produced no rewrite
// are left addressing the original source.
//
// If no file needs rewriting the function returns (nil, nil),
// meaning "forward the compile unchanged" — including when the
// package uses proven only indirectly (e.g. as a dependency of a
// dependency but with no direct proven.That calls of its own).
func rewritePlan(toolArgs []string, sources []string, files []*ast.File, fset *token.FileSet) (*Plan, error) {
	substitutions := map[string]string{}
	var tempPaths []string

	for i, src := range sources {
		imp := collectImports(files[i])
		rewritten, changed, err := rewriteSource(src, files[i], fset, imp)
		if err != nil {
			return nil, fmt.Errorf("rewrite %s: %w", src, err)
		}
		if !changed {
			continue
		}
		tmpPath, err := writeRewrittenTemp(src, rewritten)
		if err != nil {
			// Best-effort cleanup of what we wrote so far.
			for _, p := range tempPaths {
				os.Remove(p)
			}
			return nil, fmt.Errorf("write rewritten %s: %w", src, err)
		}
		tempPaths = append(tempPaths, tmpPath)
		substitutions[src] = tmpPath
	}

	if len(substitutions) == 0 {
		return nil, nil
	}

	newArgs := make([]string, len(toolArgs))
	for i, a := range toolArgs {
		if sub, ok := substitutions[a]; ok {
			newArgs[i] = sub
		} else {
			newArgs[i] = a
		}
	}
	cleanup := func() {
		for _, p := range tempPaths {
			os.Remove(p)
		}
	}
	return &Plan{NewArgs: newArgs, Cleanup: cleanup}, nil
}

// writeRewrittenTemp writes the rewritten bytes of origPath to a
// fresh temp file and returns its absolute path. The temp file's
// name preserves the original file's basename so cmd/compile's
// error messages (if any) look familiar; the directory is a
// dedicated per-call mkdir-ed subtree to avoid collisions when
// several compiles run concurrently.
func writeRewrittenTemp(origPath string, content []byte) (string, error) {
	dir, err := os.MkdirTemp("", "proven-rewrite-*")
	if err != nil {
		return "", err
	}
	path := dir + string(os.PathSeparator) + filepathBase(origPath)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return path, nil
}

// filepathBase returns the last path component of p. Inlined so
// the rewriter doesn't take a dependency on path/filepath for a
// one-liner; behavior matches filepath.Base for the OS-native
// separator.
func filepathBase(p string) string {
	sep := string(os.PathSeparator)
	if i := lastIndex(p, sep); i >= 0 {
		return p[i+1:]
	}
	return p
}

func lastIndex(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// dischargeDiagnostics converts a single CallDischarge into one
// Diagnostic per missing predicate per parameter, using fset to
// resolve the call site's position to (file, line, col).
//
// The message form —
//
//	proven: undischarged predicate <pred> on parameter <idx> of <callee>
//
// mirrors Go's compile-diagnostic phrasing closely enough for
// editor click-through to work and is explicit about which
// predicate is missing so the fix is obvious. currentPkg is the
// import path of the package being analyzed; predicates defined
// there render as bare names, imported ones keep a short package
// qualifier. Cross-package callees likewise render with their
// short package prefix so readers can tell where the obligation
// originates.
func dischargeDiagnostics(d CallDischarge, fset *token.FileSet, currentPkg string) []Diagnostic {
	pos := fset.Position(d.Pos)
	callee := d.CalleeKey
	if d.CalleePkg != "" && d.CalleePkg != currentPkg {
		callee = lastPathSegment(d.CalleePkg) + "." + d.CalleeKey
	}
	var out []Diagnostic
	for _, p := range d.Params {
		for _, missing := range p.Missing {
			out = append(out, Diagnostic{
				File: pos.Filename,
				Line: pos.Line,
				Col:  pos.Column,
				Msg: fmt.Sprintf(
					"proven: undischarged predicate %s on parameter %d of %s",
					predicateLabel(missing, currentPkg), p.ParamIdx, callee,
				),
			})
		}
	}
	return out
}

// predicateLabel renders a Predicate for human consumption in a
// diagnostic. Same-package predicates (Pkg empty or matching the
// currently-compiled package's import path) use the bare name;
// imported predicates include a short package qualifier (last
// path segment) so readers can tell cross-package dependencies
// apart from local ones.
func predicateLabel(p Predicate, currentPkg string) string {
	if p.Pkg == "" || p.Pkg == currentPkg {
		return p.Name
	}
	return lastPathSegment(p.Pkg) + "." + p.Name
}
