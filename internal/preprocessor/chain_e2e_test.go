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

// TestChainAndThen_ViaEnvPassthrough drives proven with a real
// `-toolexec=<proven> --and-then env` invocation and verifies the
// preprocessor still discharges its obligations correctly.
//
// `env`, given a command as its args, simply execs that command —
// so the chain degenerates to "proven analyzes + rewrites, env
// forwards to compile". If the chain wiring is wrong (successor
// dropped, tool misidentified, args clobbered), the build either
// fails outright or surfaces a raw undischarged-obligation diagnostic
// that the non-chained path handles cleanly.
//
// This is the minimum viable e2e: one real preprocessor plus one
// trivial passthrough. Multi-preprocessor chains use the same
// mechanism, so if this passes, N-hop chains with proven at the
// front are wired correctly.
func TestChainAndThen_ViaEnvPassthrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /usr/bin/env on Windows")
	}
	envPath, err := exec.LookPath("env")
	if err != nil {
		t.Skipf("no `env` binary on PATH: %v", err)
	}

	tmp := t.TempDir()

	// A fixture that exercises a precondition + a matching guard so
	// the preprocessor actually has obligations to discharge. If the
	// chain drops proven's rewriting step, the atCompileTime linker
	// symbol remains unresolved and the build fails loudly.
	src := `package fixture

import (
	"fmt"

	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(n int) bool { return n > 0 }

func needPositive(n int) {
	proven.That(n, isPositive)
	fmt.Println(n)
}

func main() {
	x := 5
	if isPositive(x) {
		needPositive(x)
	}
}
`
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	goMod := fmt.Sprintf(`module fixture

go 1.26

require github.com/GiGurra/proven v0.0.0

replace github.com/GiGurra/proven => %s
`, repoRoot())
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "GOCACHE="+goCache)

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = tmp
	tidy.Env = env
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	// -toolexec takes a single program-and-args string; Go tokenizes
	// it on whitespace and prepends it to every tool invocation.
	// proven parses its own args up to the first --and-then, then
	// hands the remainder (env + the real Go tool + tool args) off.
	toolexec := provenBin + " --and-then " + envPath
	cmd := exec.Command("go", "build", "-toolexec", toolexec, "./...")
	cmd.Dir = tmp
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	got := strings.TrimSpace(string(out))
	if err != nil {
		t.Fatalf("chained go build failed: %v\n---\n%s", err, got)
	}
}
