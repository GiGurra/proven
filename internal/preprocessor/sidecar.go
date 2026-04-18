package preprocessor

// Phase 6: per-package summary sidecars.
//
// After a clean analysis of package B, the preprocessor writes
// B's PackageSummary as JSON to a sidecar next to B's output .a
// file in the Go build's per-compile tempdir. A later compile of
// package A that imports B reads the sidecar via A's -importcfg
// argument, which maps import paths to their compiled .a paths.
// The result is a map[importPath]*PackageSummary that the
// analyzer consults for cross-package callees and inference
// rules.
//
// Sidecars live alongside `_pkg_.a` as `_pkg_.proven.json`.
// Writing is best-effort from the perspective of the compile —
// if the sidecar cannot be written the compile forwards anyway,
// because a missing sidecar downstream just means "no
// cross-package obligation information for this import", which
// is indistinguishable from a package that legitimately has no
// obligations.
//
// Scope (documented). The sidecar lives in the build tempdir,
// not GOCACHE. It persists only for the duration of one
// `go build`. If Go reuses a cached .a across builds (toolexec
// part of the cache key, but compile action skipped on cache
// hit), its sidecar will not be present in the downstream
// build's tempdir. Phase X will replace this with GOCACHE-
// adjacent storage so summaries survive across builds.

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// sidecarFileName is the fixed basename for the JSON summary
// sibling to `_pkg_.a`.
const sidecarFileName = "_pkg_.proven.json"

// writeSummarySidecar writes sum as JSON to the sidecar path
// derived from outputAPath (the -o flag of the compile). If the
// directory does not exist or the write fails, the error is
// returned — callers decide whether to abort or forward. A
// successful write returns (path, nil); the caller may record it
// for cleanup or leave it to Go's tempdir lifecycle.
func writeSummarySidecar(outputAPath string, sum *PackageSummary) (string, error) {
	if outputAPath == "" {
		return "", errors.New("no -o output path")
	}
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return "", err
	}
	path := sidecarPath(outputAPath)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// sidecarPath returns the summary sidecar path for a compile's
// -o output. The implementation is trivial but centralized so
// reader and writer agree on the scheme.
func sidecarPath(outputAPath string) string {
	return filepath.Join(filepath.Dir(outputAPath), sidecarFileName)
}

// readImportSummaries parses the importcfg file passed to the
// compiler and returns a map of importPath → summary for every
// packagefile entry whose sibling sidecar exists and parses. A
// missing or malformed sidecar yields no entry for that import —
// the caller's analyzer treats the callee as having no recorded
// obligations, which is the correct (and safe) behavior for
// external packages we have not scanned.
//
// importcfg line format (from cmd/go):
//
//	# comment
//	packagefile <importpath>=<absolute .a path>
//
// plus `packageshadow`, `modinfo`, etc., which we ignore.
func readImportSummaries(importcfgPath string) (map[string]*PackageSummary, error) {
	if importcfgPath == "" {
		return nil, nil
	}
	f, err := os.Open(importcfgPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]*PackageSummary)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		path, aFile, ok := parsePackagefile(line)
		if !ok {
			continue
		}
		sum, err := loadSidecar(sidecarPath(aFile))
		if err != nil {
			continue
		}
		out[path] = sum
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parsePackagefile matches a single packagefile line in an
// importcfg file. Returns the import path, the .a path, and
// whether the line matched. Comments, blank lines, and
// unrelated directives return ok=false.
func parsePackagefile(line string) (string, string, bool) {
	const prefix = "packagefile "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	path, aFile, ok := strings.Cut(line[len(prefix):], "=")
	if !ok {
		return "", "", false
	}
	return path, aFile, true
}

// loadSidecar reads the JSON summary at path and unmarshals it
// into a PackageSummary. Treats "file does not exist" as a
// non-error so callers can iterate importcfg entries without
// branching.
func loadSidecar(path string) (*PackageSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sum PackageSummary
	if err := json.Unmarshal(data, &sum); err != nil {
		return nil, err
	}
	// Ensure Funcs is non-nil so lookups don't need a nil guard.
	if sum.Funcs == nil {
		sum.Funcs = make(map[string]*FuncSummary)
	}
	return &sum, nil
}
