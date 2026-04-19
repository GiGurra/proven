package preprocessor

// Phase 5b: zero-cost rewriting of discharged proven.That /
// proven.Returns calls.
//
// The atCompileTime linker stub makes every surviving proven.That
// / proven.Returns call a no-op at runtime, but the call itself is
// still emitted: a closure allocation for the func() argument, a
// linkname-resolved dispatch, and a useless return. Phase 5b erases
// the calls at the source level before the compile tool sees them,
// so the compiled binary has no residual cost at sites the
// analyzer already cleared.
//
// The erasure is driven by the AST but applied to the raw source
// bytes: every edit is length-preserving, so the compile tool's
// error messages (and cmd/vet, and any IDE that later parses the
// temp file) still report file:line:col positions that match the
// user's original source exactly. This is the main reason not to
// use go/printer here — reprinting necessarily shifts columns.
//
// Rules of the rewrite:
//
//   - proven.That(...) used as a statement (ast.ExprStmt wrapping
//     the call): blank the entire statement's byte span. Newlines
//     and carriage returns are preserved so line numbering stays
//     intact; every other character becomes a space.
//
//   - proven.Returns(v, preds...) used as an expression: blank the
//     wrapper (`proven.Returns(`, the comma, the predicates, and
//     the closing `)`) but leave v's bytes in place. This turns
//     `proven.Returns(42, p)` into `                42     ` — the
//     `42` is exactly where it was, callers downstream see the
//     same type and value, and the compile sees a bare literal.
//
// Nesting composes: `proven.Returns(proven.Returns(42, p), q)`
// collapses to `42` after two passes (innermost first, recomputed
// against the current buffer state).

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
)

// rewriteSource reads the bytes of path, erases every discharged
// proven.That / proven.Returns call in f, and returns the
// modified bytes. The bool return reports whether any edit was
// made: false means the caller should pass the original path to
// the compile tool unchanged. A non-nil error comes only from I/O.
//
// The caller is responsible for ensuring that every call-site
// obligation in the package has been discharged before invoking
// rewriteSource — the rewriter makes no attempt to verify that
// condition and will happily erase an unproven proven.That.
func rewriteSource(path string, f *ast.File, fset *token.FileSet, imp *importInfo) ([]byte, bool, error) {
	trustAlias := imp.aliasFor(trustImportPath)
	if imp.provenAlias == "" && trustAlias == "" {
		return nil, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	edits, usedAliases := collectRewrites(f, fset, imp, trustAlias)
	if len(edits) == 0 {
		return nil, false, nil
	}
	applyRewrites(raw, edits)

	// Every proven.That / proven.Returns / trust.That use in the
	// file has just been blanked; if those were the only
	// references to the respective package, the compile would
	// reject "imported and not used". Appended sentinels on a
	// brand-new line at the end of the file anchor the imports
	// without perturbing the earlier line numbering — any error
	// the compile emits at the real-code lines maps back to the
	// user's source column-for-column.
	if imp.provenAlias != "" {
		raw = appendImportSentinel(raw, imp.provenAlias+".PredicateName")
	}
	if trustAlias != "" {
		// trust.That is generic, so the bare symbol cannot be
		// referenced without instantiation. struct{} is the
		// cheapest zero-sized type parameter that always satisfies
		// the `any` constraint.
		raw = appendImportSentinel(raw, trustAlias+".That[struct{}]")
	}
	// Third-party predicate packages referenced only inside erased
	// calls fall into the same trap: the call that used them is
	// gone, so the import becomes "imported and not used". For each
	// such alias we saw named inside an erased span we emit one
	// sentinel. Ordered for determinism so regression tests are
	// stable across runs.
	for _, alias := range sortedAliasKeys(usedAliases) {
		raw = appendImportSentinel(raw, alias+"."+usedAliases[alias])
	}

	// Prepend a file-level `//line <user-path>:1` directive so DWARF
	// and compile/vet diagnostics record the user's original source
	// path, not the preprocessor's tempdir. Without this, IDE
	// breakpoints set against the user's file don't match the
	// binary's debug info and never fire. Proven's rewrites are
	// already length-preserving, so a single file-level directive is
	// sufficient — no per-edit resets are needed. The absolute path
	// is what debuggers and go tooling expect; we fall back to the
	// original (typically already absolute from the compile argv) if
	// filepath.Abs errors.
	absPath, absErr := filepath.Abs(path)
	if absErr != nil {
		absPath = path
	}
	prefix := []byte("//line " + absPath + ":1\n")
	raw = append(prefix, raw...)

	return raw, true, nil
}

// sortedAliasKeys returns the keys of m in lexicographic order so
// the emitted sentinel ordering is deterministic — important for
// byte-identical test fixtures and predictable diff output.
func sortedAliasKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// appendImportSentinel appends `var _ = <rhs>` on a fresh line
// so the import stays in use even when every call was erased.
// Callers choose rhs so that non-generic symbols can be
// referenced bare (`alias.Name`) while generics require an
// explicit type instantiation (`alias.Name[struct{}]`); both are
// side-effect-free at runtime because the program only takes the
// value, never calls it.
func appendImportSentinel(raw []byte, rhs string) []byte {
	// Trailing newline is not guaranteed on user sources; add one
	// before the sentinel to keep the result well-formed.
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		raw = append(raw, '\n')
	}
	raw = append(raw, []byte("var _ = "+rhs+"\n")...)
	return raw
}

// rewriteEdit describes one erasure site. For a proven.That
// statement, argStart == argEnd == stmtEnd (the whole span is
// blanked). For a proven.Returns call, [argStart, argEnd) is the
// byte range of the value expression that must survive; the rest
// of [start, end) is blanked.
type rewriteEdit struct {
	start, end       int
	argStart, argEnd int
}

// collectRewrites walks f and returns a list of rewriteEdit
// entries for every call the rewriter erases, plus a map of
// non-proven/non-trust import aliases referenced inside those
// erased spans (alias → first-seen selector name). The map lets
// the caller emit import sentinels for third-party predicate
// packages that would otherwise end up "imported and not used"
// once the erasure runs.
//
// Erased shapes:
//
//   - proven.That(...) used as a statement — whole ExprStmt
//     blanked.
//   - proven.Returns(v, ...) used as an expression — wrapper
//     blanked, v's bytes preserved at their original column.
//   - trust.That(v, ...) — same expression-level erasure as
//     proven.Returns.
//
// Ordering (innermost first) is enforced by applyRewrites at
// apply time; collection here is top-down so nested calls are
// all captured. An empty alias ("" for a package not imported
// by the file) makes the corresponding branch a no-op via
// isSel's alias check.
func collectRewrites(f *ast.File, fset *token.FileSet, imp *importInfo, trustAlias string) ([]rewriteEdit, map[string]string) {
	provenAlias := imp.provenAlias
	var edits []rewriteEdit
	usedAliases := map[string]string{}
	recordUsed := func(args []ast.Expr) {
		for _, arg := range args {
			collectAliasRefs(arg, imp, provenAlias, trustAlias, usedAliases)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ExprStmt:
			call, ok := node.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			// Bare fact-assertion statements — proven.That, and the
			// purely-passthrough trust.That — are erased whole. If
			// we erased only the wrapper the surviving arg[0] would
			// become an invalid bare expression statement ("x is
			// not used"). Blanking the whole ExprStmt is safe
			// because neither call has observable side effects
			// under the preprocessor contract: trust.That is a no-op
			// at runtime, and proven.That's runtime behavior is
			// gated behind the preprocessor link symbol.
			wholeStmt := isSel(call, provenAlias, "That") ||
				isSel(call, trustAlias, "That")
			if !wholeStmt {
				return true
			}
			start := fset.Position(node.Pos()).Offset
			end := fset.Position(node.End()).Offset
			edits = append(edits, rewriteEdit{
				start: start, end: end,
				argStart: end, argEnd: end,
			})
			recordUsed(call.Args)
			// Do NOT descend: a descent would re-match the inner
			// CallExpr (trust.That / proven.Returns) and add a
			// second, overlapping edit that preserves arg[0]. Under
			// applyRewrites' layered write the second edit would
			// win for its range, restoring arg[0] into a now-bare
			// expression position and producing invalid Go. Any
			// nested proven.Returns inside the args is already
			// covered by the outer whole-statement blank.
			return false
		case *ast.CallExpr:
			switch {
			case isSel(node, provenAlias, "Returns"):
			case isSel(node, trustAlias, "That"):
			case isSel(node, trustAlias, "Returns"):
			default:
				return true
			}
			if len(node.Args) == 0 {
				return true
			}
			callStart := fset.Position(node.Pos()).Offset
			callEnd := fset.Position(node.End()).Offset
			argStart := fset.Position(node.Args[0].Pos()).Offset
			argEnd := fset.Position(node.Args[0].End()).Offset
			edits = append(edits, rewriteEdit{
				start: callStart, end: callEnd,
				argStart: argStart, argEnd: argEnd,
			})
			// Only the wrapper is erased — args[0] survives — so
			// track aliases used in args[1:], which become dead.
			if len(node.Args) > 1 {
				recordUsed(node.Args[1:])
			}
			return true
		}
		return true
	})
	return edits, usedAliases
}

// collectAliasRefs walks expr and records, for every SelectorExpr
// whose receiver is an import-alias Ident (other than proven or
// trust), the first selector name we see. The alias → name map lets
// the rewriter emit one import sentinel per package whose only
// references were inside erased call spans.
//
// Receivers that look like import aliases syntactically but are not
// imports in this file (a local variable named the same as a Go
// package, or a field selector rooted in a local) are skipped: the
// generated sentinel would land at file scope where no such binding
// exists and produce an "undefined" compile error.
func collectAliasRefs(expr ast.Expr, imp *importInfo, provenAlias, trustAlias string, out map[string]string) {
	ast.Inspect(expr, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == provenAlias || id.Name == trustAlias {
			return true
		}
		if imp == nil {
			return true
		}
		if _, isImport := imp.aliases[id.Name]; !isImport {
			return true
		}
		if _, seen := out[id.Name]; seen {
			return true
		}
		out[id.Name] = sel.Sel.Name
		return true
	})
}

// applyRewrites mutates raw in place, blanking the non-surviving
// bytes of each edit. Edits are applied in order of descending
// start offset so innermost edits run first — when a later
// (outer) edit reads bytes in its surviving argument span, it
// reads whatever the inner edits have already written there.
//
// All edits are length-preserving (same byte count in, same byte
// count out), so positions never shift and non-overlapping edits
// commute. The ordering only matters where edits nest.
func applyRewrites(raw []byte, edits []rewriteEdit) {
	sortEditsInnermostFirst(edits)
	for _, e := range edits {
		blankRange(raw, e.start, e.argStart)
		blankRange(raw, e.argEnd, e.end)
	}
}

// sortEditsInnermostFirst orders edits so that a nested edit
// runs before the edit that encloses it. Innermost has a strictly
// larger start than its enclosing outer; sorting by start
// descending is enough. For unrelated (non-nested) edits the
// order is irrelevant since they do not overlap.
func sortEditsInnermostFirst(edits []rewriteEdit) {
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j-1].start < edits[j].start; j-- {
			edits[j-1], edits[j] = edits[j], edits[j-1]
		}
	}
}

// blankRange replaces raw[start:end] with spaces, preserving any
// newline or carriage return bytes so line numbering does not
// drift.
func blankRange(raw []byte, start, end int) {
	if start >= end {
		return
	}
	for i := start; i < end; i++ {
		switch raw[i] {
		case '\n', '\r':
			// leave line terminator as-is
		default:
			raw[i] = ' '
		}
	}
}

// isSel reports whether call is an `<alias>.<name>` selector
// call. An empty alias never matches, so callers can pass the
// aliases for packages the file does not import and the
// corresponding rewrite branch silently skips.
func isSel(call *ast.CallExpr, alias, name string) bool {
	if alias == "" {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != alias {
		return false
	}
	return sel.Sel.Name == name
}
