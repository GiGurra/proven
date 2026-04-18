package preprocessor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootForTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// TestPlanProvenStub_FromRealSource exercises planProvenStub against
// the actual pkg/proven source files. It is the tightest unit check
// we have that AST scanning finds the //go:linkname declaration and
// emits a stub file whose content matches the expected shape. The
// e2e harness covers whether the stub actually links; this test
// isolates the AST-parsing side of the pipeline.
func TestPlanProvenStub_FromRealSource(t *testing.T) {
	root := repoRootForTest(t)
	provenDir := filepath.Join(root, "pkg", "proven")
	entries, err := os.ReadDir(provenDir)
	if err != nil {
		t.Fatalf("read pkg/proven: %v", err)
	}
	var sources []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		sources = append(sources, filepath.Join(provenDir, e.Name()))
	}
	if len(sources) == 0 {
		t.Fatalf("no .go files under pkg/proven")
	}

	stub, cleanup, err := provenStubFromSources(sources)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("provenStubFromSources: %v", err)
	}
	if stub == "" {
		t.Fatal("expected a stub file, got empty path")
	}

	data, err := os.ReadFile(stub)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		"package proven",
		`import _ "unsafe"`,
		"//go:linkname provenAtCompileTimeImpl " + provenLinkSymbol,
		"func provenAtCompileTimeImpl",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stub missing %q in:\n%s", want, got)
		}
	}
}

// TestPlanProvenStub_UnrelatedPackage — when no //go:linkname for
// _proven_atCompileTime is present in the provided sources, the
// handler must silently return no extras. The absence of the symbol
// in a random package is not an error we want to raise at compile
// time.
func TestPlanProvenStub_UnrelatedPackage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unrelated.go")
	src := `package unrelated

func Foo() {}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	stub, cleanup, err := provenStubFromSources([]string{path})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("provenStubFromSources: %v", err)
	}
	if stub != "" {
		t.Errorf("expected no stub for unrelated package, got %q", stub)
	}
}
