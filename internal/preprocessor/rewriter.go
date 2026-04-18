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
	if imp.provenAlias == "" {
		return nil, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	edits := collectRewrites(f, fset, imp.provenAlias)
	if len(edits) == 0 {
		return nil, false, nil
	}
	applyRewrites(raw, edits)

	// Every proven.That / proven.Returns use in the file has just
	// been blanked; if those were the only references to the
	// proven package, the compile would reject "imported and not
	// used". An appended sentinel on a brand-new line anchors the
	// import without perturbing the earlier line numbering — any
	// error the compile emits at the real-code lines maps back to
	// the user's source column-for-column.
	raw = appendImportSentinel(raw, imp.provenAlias)
	return raw, true, nil
}

// appendImportSentinel appends a blank-identifier reference to
// proven so the import stays in use even when every proven.That /
// proven.Returns has been erased. proven.PredicateName is
// exported, non-generic, has no side effects, and costs nothing
// at runtime — taking its address does not invoke it — so this is
// the cheapest durable anchor available in pkg/proven.
func appendImportSentinel(raw []byte, provenAlias string) []byte {
	// Trailing newline is not guaranteed on user sources; add one
	// before the sentinel to keep the result well-formed.
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		raw = append(raw, '\n')
	}
	raw = append(raw, []byte("var _ = "+provenAlias+".PredicateName\n")...)
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
// entries for every proven.That / proven.Returns call. Ordering
// (innermost first) is enforced by applyRewrites at apply time;
// collection here is top-down so nested calls are all captured.
func collectRewrites(f *ast.File, fset *token.FileSet, provenAlias string) []rewriteEdit {
	var edits []rewriteEdit
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ExprStmt:
			call, ok := node.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			if !isProvenSel(call, provenAlias, "That") {
				return true
			}
			start := fset.Position(node.Pos()).Offset
			end := fset.Position(node.End()).Offset
			edits = append(edits, rewriteEdit{
				start: start, end: end,
				argStart: end, argEnd: end,
			})
			// Fall through so a nested proven.Returns inside the
			// args of an about-to-be-erased proven.That is still
			// collected; leaving it un-erased would waste bytes
			// but it is overwritten by the outer blank pass.
			// Descending costs nothing and keeps behavior simple.
			return true
		case *ast.CallExpr:
			if !isProvenSel(node, provenAlias, "Returns") {
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
			return true
		}
		return true
	})
	return edits
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

// isProvenSel reports whether call is a `<provenAlias>.<name>` call.
func isProvenSel(call *ast.CallExpr, provenAlias, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != provenAlias {
		return false
	}
	return sel.Sel.Name == name
}
