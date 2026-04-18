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

var (
	provenBin string
	goCache   string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "proven-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create tempdir:", err)
		os.Exit(1)
	}
	provenBin = filepath.Join(dir, "proven")

	// Isolated GOCACHE for every fixture build. Go's build cache key
	// does not include toolexec behavior: a prior non-toolexec build
	// of pkg/proven leaves a stub-less artifact in the host cache
	// which fixtures would then reuse, failing to link with
	// "relocation target _proven_atCompileTime not defined". A prior
	// toolexec build would pollute the other direction with a
	// stub-containing artifact that clashes with proventest. Isolation
	// makes the harness deterministic regardless of host cache state.
	goCache = filepath.Join(dir, "gocache")
	if err := os.MkdirAll(goCache, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "failed to create gocache:", err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	cmd := exec.Command("go", "build", "-o", provenBin, "./cmd/proven")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "failed to build cmd/proven:")
		fmt.Fprintln(os.Stderr, string(out))
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
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

	// Copy every .go source file into a mirrored tree under tmp.
	// Multi-package fixtures place subpackages in subdirectories
	// (e.g. pkg/callee.go); expected.txt lives at the fixture
	// root and is excluded here.
	if err := copyGoTree(fixtureDir, tmp); err != nil {
		t.Fatal(err)
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

// copyGoTree walks src and replicates every .go source file at
// the same relative path under dst. Non-.go files (expected.txt,
// any extraneous artifacts the fixture author might leave behind)
// are skipped so the fixture's tmp tree is precisely a Go module
// source tree. Directories are created as needed.
func copyGoTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func runIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Every fixture build uses the harness-owned GOCACHE. Anything the
	// host cache holds for pkg/proven would be either stub-less (from
	// a plain go test run) or stub-containing (from a prior e2e run),
	// and Go's cache key does not distinguish the two — the link step
	// would fail non-deterministically depending on order. The
	// isolated cache makes the harness a hermetic test of what the
	// preprocessor itself produces.
	cmd.Env = append(os.Environ(), "GOCACHE="+goCache)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
