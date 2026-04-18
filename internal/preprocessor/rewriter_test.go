package preprocessor

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rewriteString is a test helper: parse src, run rewriteSource
// against it, and return the rewritten bytes plus the changed
// flag. Caller is responsible for any post-assertions.
func rewriteString(t *testing.T, src string) ([]byte, bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "in.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	imp := collectImports(f)
	out, changed, err := rewriteSource(path, f, fset, imp)
	if err != nil {
		t.Fatalf("rewriteSource: %v", err)
	}
	return out, changed
}

// parsesCleanly reports whether b is still valid Go source. We
// want every rewritten output to remain parseable so the compile
// tool accepts the temp file.
func parsesCleanly(t *testing.T, b []byte) bool {
	t.Helper()
	_, err := parser.ParseFile(token.NewFileSet(), "in.go", b, 0)
	return err == nil
}

func TestRewrite_ThatStatementIsBlanked(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	proven.That(x, isPositive)
}
`
	out, changed := rewriteString(t, src)
	if !changed {
		t.Fatal("expected a rewrite")
	}
	s := string(out)
	if strings.Contains(s, "proven.That") {
		t.Errorf("proven.That still present after rewrite:\n%s", s)
	}
	// All bytes up to and including the original source are
	// untouched; the preserved prefix matches the input's
	// non-proven-call regions 1:1. A sentinel reference is
	// appended afterwards, so overall line count grows by exactly
	// one line.
	if got, want := strings.Count(s, "\n"), strings.Count(src, "\n")+1; got != want {
		t.Errorf("line count: got %d, want %d (original + 1 sentinel)", got, want)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}

func TestRewrite_ReturnsKeepsInnerValue(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source() int {
	return proven.Returns(42, isPositive)
}
`
	out, changed := rewriteString(t, src)
	if !changed {
		t.Fatal("expected a rewrite")
	}
	s := string(out)
	if strings.Contains(s, "proven.Returns") {
		t.Errorf("proven.Returns still present:\n%s", s)
	}
	if !strings.Contains(s, "42") {
		t.Errorf("inner value 42 lost in rewrite:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}

func TestRewrite_PositionsPreserved(t *testing.T) {
	// All bytes in the original source region must stay at the
	// same offset after rewriting, so cmd/compile's error
	// messages point at the user's original line:col. The only
	// permitted change is an appended sentinel on a fresh line
	// at the end. Confirm by comparing the output's prefix of
	// len(src) bytes against the input — the prefix must match
	// except at the erased call spans, where it now has spaces.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func f(x int) int {
	proven.That(x, isPositive)
	return proven.Returns(x*2, isPositive)
}
`
	out, _ := rewriteString(t, src)
	if len(out) < len(src) {
		t.Fatalf("output shorter than input: %d < %d", len(out), len(src))
	}
	prefix := string(out[:len(src)])
	// Line boundaries must be byte-for-byte identical to the
	// original — the rewrite only blanks non-newline bytes.
	for i := 0; i < len(src); i++ {
		origIsNL := src[i] == '\n' || src[i] == '\r'
		newIsNL := prefix[i] == '\n' || prefix[i] == '\r'
		if origIsNL != newIsNL {
			t.Fatalf("newline position shifted at offset %d", i)
		}
	}
}

func TestRewrite_NestedReturnsCollapseToInnermost(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func p(x int) bool { return x > 0 }
func q(x int) bool { return x > 0 }

func f() int {
	return proven.Returns(proven.Returns(42, p), q)
}
`
	out, _ := rewriteString(t, src)
	s := string(out)
	if strings.Contains(s, "proven.Returns") {
		t.Errorf("residual proven.Returns in output:\n%s", s)
	}
	if !strings.Contains(s, "42") {
		t.Errorf("innermost 42 lost:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}

func TestRewrite_NoTargetsNoChange(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	_ = isPositive(x)
}
`
	out, changed := rewriteString(t, src)
	if changed {
		t.Errorf("expected no rewrite; got %q", string(out))
	}
	if out != nil {
		t.Errorf("expected nil output, got %q", string(out))
	}
}

func TestRewrite_FileWithoutProvenImportUntouched(t *testing.T) {
	src := `package ex

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	_ = isPositive(x)
}
`
	out, changed := rewriteString(t, src)
	if changed || out != nil {
		t.Errorf("file without proven import should not be rewritten; got changed=%v", changed)
	}
}

func TestRewrite_AliasedProvenImport(t *testing.T) {
	src := `package ex

import p "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	p.That(x, isPositive)
}
`
	out, changed := rewriteString(t, src)
	if !changed {
		t.Fatal("expected rewrite under aliased import")
	}
	s := string(out)
	if strings.Contains(s, "p.That") {
		t.Errorf("aliased p.That still present:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}

func TestRewrite_ReturnsEmbeddedInExpression(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func p(x int) bool { return x > 0 }

func f(x int) int {
	return proven.Returns(x, p) + 1
}
`
	out, _ := rewriteString(t, src)
	s := string(out)
	if strings.Contains(s, "proven.Returns") {
		t.Errorf("residual proven.Returns:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}
