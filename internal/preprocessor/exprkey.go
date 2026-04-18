package preprocessor

// Canonical expression keys for fact subjects.
//
// Phase 2 of the demand-driven restructure: a fact subject used to
// be a bare identifier name; after this phase it is a canonical
// expression key that can represent selector chains rooted in an
// identifier — "x", "holder.Value", "a.B.C", etc.
//
// The widening is intentionally narrow: only identifier-rooted
// selector paths are tracked. Index expressions, pointer dereferences,
// function-call results, and other non-stable subject shapes still
// decline to produce a key, matching the existing "untracked" behavior
// — they simply carry no fact identity that the analyzer can reason
// about across statements.
//
// Write events always key on the ROOT of the canonical key (the
// leftmost identifier). A mutation to any field of x — `x = ...`,
// `x.F = ...`, `&x`, `x[i] = ...` — invalidates every fact whose key
// is rooted at x. This matches the pre-Phase-2 forgetLHS semantics:
// writing to any reachable part of the root conservatively forgets
// all predicates about every path rooted there.

import "go/ast"

// exprKey canonicalizes an expression into a fact-subject key, or
// returns ("", false) if the expression is not a shape the analyzer
// tracks. The tracked shapes are:
//
//   - Identifier  — "x" (blank identifier "_" is rejected).
//   - Selector    — "a.B.C", provided the left-most operand of the
//                   selector chain is itself a tracked shape and not
//                   an import-alias reference.
//
// The importInfo argument, if non-nil, is consulted to reject the
// `pkg.Foo` shape where pkg is an imported package — package-scoped
// names are not local variables and carry no flow-sensitive facts.
func exprKey(e ast.Expr, imp *importInfo) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "" || x.Name == "_" {
			return "", false
		}
		// Predeclared constant identifiers are not trackable subjects:
		// they carry a value but no name a fact can be established
		// against. Rejecting them here keeps bindArgForCheck from
		// short-circuiting into exprKey when a caller passes `nil`,
		// `true`, or `false` — the literal evaluator handles those
		// shapes instead, producing virtual facts where they apply.
		switch x.Name {
		case "nil", "true", "false", "iota":
			return "", false
		}
		if imp != nil {
			if _, isImport := imp.aliases[x.Name]; isImport {
				return "", false
			}
		}
		return x.Name, true
	case *ast.SelectorExpr:
		inner, ok := exprKey(x.X, imp)
		if !ok {
			return "", false
		}
		return inner + "." + x.Sel.Name, true
	}
	return "", false
}

// exprKeyRoot returns the root identifier of a canonical expression
// key — everything before the first dot, or the whole key for a bare
// identifier. Used by the resolver's Write-barrier matching: a Write
// on root R invalidates every fact whose key starts at R.
func exprKeyRoot(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == '.' {
			return key[:i]
		}
	}
	return key
}
