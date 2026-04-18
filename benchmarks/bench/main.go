// Command bench measures end-to-end preprocessor build time on
// benchmarks/corpus. First cut: wall time for a clean build (fresh
// GOCACHE) and a no-op rebuild. Expand with deeper instrumentation
// once per-invocation numbers start mattering.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench: could not find repo root:", err)
		os.Exit(1)
	}
	corpus := filepath.Join(repoRoot, "benchmarks", "corpus")
	if _, err := os.Stat(corpus); err != nil {
		fmt.Fprintln(os.Stderr, "bench: corpus not found at", corpus)
		os.Exit(1)
	}

	// Build the preprocessor binary into a tempdir we own, so the
	// bench runs against the current checkout without depending on
	// whatever happens to be on $PATH.
	binDir, err := os.MkdirTemp("", "proven-bench-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench: tempdir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(binDir)
	binary := filepath.Join(binDir, "proven")
	if out, err := runIn(repoRoot, nil, "go", "build", "-o", binary, "./cmd/proven"); err != nil {
		fmt.Fprintln(os.Stderr, "bench: build proven:", err)
		fmt.Fprintln(os.Stderr, out)
		os.Exit(1)
	}

	fmt.Printf("bench: proven binary at %s\n", binary)
	fmt.Printf("bench: corpus at       %s\n", corpus)
	fmt.Printf("bench: cpus=%d\n\n", runtime.NumCPU())

	// Isolated GOCACHE so the bench result does not depend on
	// whatever the host cache contains. Every timed sub-run uses the
	// SAME cache to model clean → incremental behavior.
	cache, err := os.MkdirTemp("", "proven-bench-cache-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench: cache dir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(cache)

	env := []string{"GOCACHE=" + cache}

	// Build 1: clean (fresh cache).
	reportRun("clean build (empty GOCACHE)", func() error {
		_, err := runIn(corpus, env, "go", "build", "-toolexec="+binary, "./...")
		return err
	})

	// Build 2: no-op rebuild (hot cache, no source changes).
	reportRun("no-op rebuild (hot cache)", func() error {
		_, err := runIn(corpus, env, "go", "build", "-toolexec="+binary, "./...")
		return err
	})
}

func reportRun(label string, fn func() error) {
	start := time.Now()
	err := fn()
	d := time.Since(start)
	if err != nil {
		fmt.Printf("%-34s FAIL (%s): %v\n", label, d, err)
		return
	}
	fmt.Printf("%-34s %s\n", label, d)
}

func runIn(dir string, extraEnv []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// findRepoRoot walks upward from cwd until it sees the top-level
// go.mod for the parent proven module (the one with
// `module github.com/GiGurra/proven`). Returns the containing path.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			if isTopLevelModule(data) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("reached filesystem root without finding top-level go.mod")
		}
		dir = parent
	}
}

func isTopLevelModule(data []byte) bool {
	// Minimal: the top-level module declaration is the first non-
	// blank non-comment line starting with "module ". Accept only
	// "module github.com/GiGurra/proven" so we don't match the
	// corpus's own go.mod.
	for _, line := range splitLines(data) {
		line = trimLeft(line)
		if len(line) == 0 || line[0] == '/' {
			continue
		}
		if startsWith(line, "module ") {
			rest := trim(line[len("module "):])
			return rest == "github.com/GiGurra/proven"
		}
		return false
	}
	return false
}

// tiny local helpers — keep bench free of dependencies beyond stdlib.

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func trimLeft(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

func trim(s string) string {
	s = trimLeft(s)
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
