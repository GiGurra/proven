package preprocessor

// Phase 5: per-user-package scan + analyze + diagnose.
//
// For every compile invocation whose target is not pkg/proven, we
// parse the source files the compiler received, build the
// obligation summary (Phase 2), and analyze each caller's body
// for undischarged obligations (Phases 3–4). When any call-site
// obligation remains unproven, the preprocessor emits
// Go-standard diagnostics and the build fails without reaching
// the real compile tool. When every obligation is discharged —
// or the package has no obligations at all — the compile
// forwards unchanged; the atCompileTime linker stub continues to
// make runtime proven.That / proven.Returns calls no-ops.
//
// Rewriting discharged calls to eliminate the closure-allocation
// overhead is a follow-on (Phase 5b) tracked in the roadmap; the
// correctness-critical piece is establishing that a program
// which compiles under the preprocessor has every proven.That
// obligation proved.

import (
	"fmt"
	"go/token"
)

// planUserPackage handles the compile of any package other than
// pkg/proven. It scans the package for obligations, analyzes every
// function body against that summary, and returns either:
//
//   - nil Plan, nil error: forward the compile unchanged (either
//     the package has no obligations, or every call-site
//     obligation is discharged);
//
//   - non-nil Plan with Diags set: the build fails before the
//     compile runs, with one diagnostic per missing predicate per
//     call site;
//
//   - non-nil error: an infrastructure failure (a source file
//     failed to parse). The Go toolchain will surface the same
//     error when it reaches the real compile, so we forward it
//     upstream as "proven: ...".
//
// Packages with no .go source files in their argv (unusual but
// legal — cgo stubs, auto-generated stdlib artifacts) short-
// circuit immediately.
func planUserPackage(pkgPath string, toolArgs []string) (*Plan, error) {
	sources := compileSourceFiles(toolArgs)
	if len(sources) == 0 {
		return nil, nil
	}

	_, discharges, fset, err := AnalyzePackage(pkgPath, sources)
	if err != nil {
		return nil, fmt.Errorf("analyze %s: %w", pkgPath, err)
	}

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
	return nil, nil
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
// qualifier.
func dischargeDiagnostics(d CallDischarge, fset *token.FileSet, currentPkg string) []Diagnostic {
	pos := fset.Position(d.Pos)
	var out []Diagnostic
	for _, p := range d.Params {
		for _, missing := range p.Missing {
			out = append(out, Diagnostic{
				File: pos.Filename,
				Line: pos.Line,
				Col:  pos.Column,
				Msg: fmt.Sprintf(
					"proven: undischarged predicate %s on parameter %d of %s",
					predicateLabel(missing, currentPkg), p.ParamIdx, d.CalleeKey,
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
