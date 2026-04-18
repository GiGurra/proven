package preprocessor

import (
	"go/ast"
	"go/parser"
	"testing"
)

func parseExprHelper(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return e
}

// TestExprKey_TrackedShapes covers the shapes that canonicalize to a
// valid key: identifiers and identifier-rooted selector chains.
func TestExprKey_TrackedShapes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"x", "x"},
		{"holder.Value", "holder.Value"},
		{"a.B.C", "a.B.C"},
		{"root.Child.Leaf.Name", "root.Child.Leaf.Name"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			got, ok := exprKey(parseExprHelper(t, c.src), nil)
			if !ok {
				t.Fatalf("exprKey(%q) returned !ok", c.src)
			}
			if got != c.want {
				t.Fatalf("exprKey(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestExprKey_UntrackedShapes covers shapes the analyzer explicitly
// refuses to key — it declines with (_, false) so no fact identity
// is ever established on them.
func TestExprKey_UntrackedShapes(t *testing.T) {
	// "nil" is an Ident so exprKey returns ("nil", true). The
	// analyzer's nil-specific handling filters it where it matters
	// (nilCompareVar rejects a "nil" subject). We focus this test
	// on shapes that do NOT canonicalize.
	cases := []string{
		"_",
		"x[i]",
		"*p",
		"f(x)",
		"x.F()",
		"x + 1",
		"&x",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			got, ok := exprKey(parseExprHelper(t, src), nil)
			if ok {
				t.Fatalf("exprKey(%q) = (%q, true), want (_, false)", src, got)
			}
		})
	}
}

// TestExprKey_RejectsImportAlias checks that a selector rooted at an
// imported-package alias (e.g. "fmt.Println") declines to produce a
// key — import-scoped names are not tracked variables.
func TestExprKey_RejectsImportAlias(t *testing.T) {
	imp := &importInfo{
		aliases: map[string]string{"fmt": "fmt"},
	}
	e := parseExprHelper(t, "fmt.Println")
	if got, ok := exprKey(e, imp); ok {
		t.Fatalf("exprKey(%q) = (%q, true), want (_, false)", "fmt.Println", got)
	}
}

// TestExprKeyRoot confirms the root-extraction split; the resolver's
// Write barrier relies on this for prefix-rooted invalidation.
func TestExprKeyRoot(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"x", "x"},
		{"holder.Value", "holder"},
		{"a.B.C.D", "a"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			if got := exprKeyRoot(c.key); got != c.want {
				t.Fatalf("exprKeyRoot(%q) = %q, want %q", c.key, got, c.want)
			}
		})
	}
}
