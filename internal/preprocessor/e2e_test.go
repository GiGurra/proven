package preprocessor_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// End-to-end harness for the proven preprocessor.
//
// Each directory under testdata/cases is a fixture:
//
//	testdata/cases/<name>/
//	    *.go          # source files copied into a fresh tempdir
//	    expected.txt  # optional:
//	                  #   empty  -> build must succeed
//	                  #   non-empty -> build must fail AND output must
//	                  #                contain the expected text as a substring
//
// The harness:
//   1. Builds cmd/proven once (TestMain).
//   2. For each fixture, creates a tempdir, copies *.go files, writes
//      a synthesized go.mod with a local replace of github.com/GiGurra/proven,
//      runs `go mod tidy` and `go build -toolexec=<binary> ./...`, and
//      compares the result to expected.txt.
//
// This lets fixtures assert the exact compiler-level behavior of the
// preprocessor: "did my fixture build?", and "if not, did the diagnostic
// contain the right text?"

var provenBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "proven-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create tempdir:", err)
		os.Exit(1)
	}
	provenBin = filepath.Join(dir, "proven")

	cmd := exec.Command("go", "build", "-o", provenBin, "./cmd/proven")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "failed to build cmd/proven:")
		fmt.Fprintln(os.Stderr, string(out))
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestFixtures(t *testing.T) {
	casesDir := filepath.Join(repoRoot(), "testdata", "cases")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no fixtures yet")
		}
		t.Fatal(err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			runFixture(t, filepath.Join(casesDir, name))
		})
	}
}

func runFixture(t *testing.T, fixtureDir string) {
	t.Helper()
	tmp := t.TempDir()

	// Copy .go source files.
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(fixtureDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, e.Name()), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Synthesize a go.mod with a local replace for proven.
	goMod := fmt.Sprintf(`module fixture

go 1.26

require github.com/GiGurra/proven v0.0.0

replace github.com/GiGurra/proven => %s
`, repoRoot())
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runIn(tmp, "go", "mod", "tidy"); err != nil {
		t.Fatalf("go mod tidy failed: %v\n---\n%s", err, out)
	}

	out, buildErr := runIn(tmp, "go", "build", "-toolexec", provenBin, "./...")
	got := strings.TrimSpace(out)

	var want string
	if data, readErr := os.ReadFile(filepath.Join(fixtureDir, "expected.txt")); readErr == nil {
		want = strings.TrimSpace(string(data))
	}

	if want == "" {
		if buildErr != nil {
			t.Errorf("expected build to succeed; got error: %v\n---\n%s", buildErr, got)
		}
		return
	}

	if buildErr == nil {
		t.Errorf("expected build to fail (expected substring %q) but it succeeded.\noutput:\n%s", want, got)
		return
	}
	if !strings.Contains(got, want) {
		t.Errorf("build failed but output did not contain expected substring.\nwant substring:\n%s\n---\ngot:\n%s", want, got)
	}
}

func runIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
