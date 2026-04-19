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
	// The rewritten file is: a file-level `//line <path>:1`
	// directive (so DWARF records the user's source path), the
	// length-preserved original content, and an appended sentinel
	// reference. Line count grows by exactly two lines.
	if got, want := strings.Count(s, "\n"), strings.Count(src, "\n")+2; got != want {
		t.Errorf("line count: got %d, want %d (original + //line prefix + sentinel)", got, want)
	}
	if !strings.HasPrefix(s, "//line ") {
		t.Errorf("missing file-level //line prefix:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}

func TestRewrite_EmitsLineDirective(t *testing.T) {
	// The rewritten file must start with a file-level //line
	// directive whose filename is the absolute path of the user's
	// source. Without this, DWARF records the preprocessor's
	// tempdir and IDE breakpoints don't match.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) {
	proven.That(x, isPositive)
}
`
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
	out, _, err := rewriteSource(path, f, fset, imp)
	if err != nil {
		t.Fatal(err)
	}

	absPath, _ := filepath.Abs(path)
	wantPrefix := "//line " + absPath + ":1\n"
	if !strings.HasPrefix(string(out), wantPrefix) {
		firstLine := string(out)
		if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
			firstLine = firstLine[:i]
		}
		t.Errorf("missing or wrong //line prefix.\nwant: %q\ngot:  %q", wantPrefix, firstLine)
	}

	// The directive must not break parseability.
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten output does not parse:\n%s", out)
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
	// Inside the original-source region, all bytes must stay at
	// the same offset after rewriting, so cmd/compile's error
	// messages (under the //line directive) point at the user's
	// original line:col. The only permitted changes are:
	//   - A `//line <path>:1\n` directive prepended to the file,
	//     which rebases the compiler's view of line 1 onto the
	//     original source path; the directive itself is one
	//     physical line the compiler effectively skips.
	//   - An appended sentinel on a fresh line at the end.
	// Confirm by stripping the prefix, then comparing
	// len(src) bytes against the input — those must match except
	// at the erased call spans, where it now has spaces.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func f(x int) int {
	proven.That(x, isPositive)
	return proven.Returns(x*2, isPositive)
}
`
	out, _ := rewriteString(t, src)
	// Skip the file-level //line directive line.
	nlIdx := strings.IndexByte(string(out), '\n')
	if nlIdx < 0 || !strings.HasPrefix(string(out), "//line ") {
		t.Fatalf("expected rewritten output to start with a //line directive; got:\n%s", out)
	}
	body := out[nlIdx+1:]

	if len(body) < len(src) {
		t.Fatalf("output body shorter than input: %d < %d", len(body), len(src))
	}
	prefix := string(body[:len(src)])
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
