package preprocessor

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSummaryRoundtripJSON: marshal a PackageSummary with every
// meaningful field populated and read it back, asserting struct
// equality. A regression here would mean Phase 6's sidecar is
// dropping fields and downstream discharge decisions are being
// made against a lossy copy.
func TestSummaryRoundtripJSON(t *testing.T) {
	given := Predicate{Pkg: "example.com/ex", Name: "isGreaterThanZero"}
	orig := &PackageSummary{
		ImportPath: "example.com/ex",
		Funcs: map[string]*FuncSummary{
			"Target": {
				Name: "Target",
				ParamPreds: map[int][]Predicate{
					0: {{Pkg: "example.com/ex", Name: "isPositive"}},
				},
			},
			"Holder.Method": {
				Name: "Method",
				Recv: "Holder",
				ParamPreds: map[int][]Predicate{
					1: {{Pkg: "example.com/ex", Name: "isNonEmpty"}},
				},
				ReturnPreds: []Predicate{{Pkg: "example.com/ex", Name: "isPositive"}},
			},
		},
		Rules: []InferRule{
			{
				From: Predicate{Pkg: "example.com/ex", Name: "isEven"},
				To:   Predicate{Pkg: "example.com/ex", Name: "isPositive"},
			},
			{
				From:  Predicate{Pkg: "example.com/ex", Name: "isSmallPositive"},
				Given: &given,
				To:    Predicate{Pkg: "example.com/ex", Name: "isPositive"},
			},
		},
	}

	dir := t.TempDir()
	// Use sidecarPath against a fake .a path so the writer's
	// directory layout matches production.
	aPath := filepath.Join(dir, "_pkg_.a")
	path, err := writeSummarySidecar(aPath, orig)
	if err != nil {
		t.Fatalf("writeSummarySidecar: %v", err)
	}
	if path != sidecarPath(aPath) {
		t.Errorf("sidecar written at %q, want %q", path, sidecarPath(aPath))
	}
	got, err := loadSidecar(path)
	if err != nil {
		t.Fatalf("loadSidecar: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		// Marshal both sides to make any diff readable.
		a, _ := json.MarshalIndent(orig, "", "  ")
		b, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("round-trip mismatch\nwant:\n%s\ngot:\n%s", a, b)
	}
}

// TestReadImportSummaries wires the importcfg parser end-to-end
// against a realistic file laid out on disk: two packagefile
// entries (one with a sidecar, one without) plus noise lines
// (comments, unrelated directives). Only the entry with a
// present, valid sidecar should yield a summary.
func TestReadImportSummaries(t *testing.T) {
	dir := t.TempDir()
	withSidecar := filepath.Join(dir, "withA")
	withoutSidecar := filepath.Join(dir, "withoutA")
	if err := os.MkdirAll(withSidecar, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(withoutSidecar, 0o755); err != nil {
		t.Fatal(err)
	}

	wantSummary := &PackageSummary{
		ImportPath: "example.com/with",
		Funcs: map[string]*FuncSummary{
			"Target": {
				Name: "Target",
				ParamPreds: map[int][]Predicate{
					0: {{Pkg: "example.com/with", Name: "isPositive"}},
				},
			},
		},
	}
	if _, err := writeSummarySidecar(filepath.Join(withSidecar, "_pkg_.a"), wantSummary); err != nil {
		t.Fatal(err)
	}

	importcfg := filepath.Join(dir, "importcfg")
	contents := "# importcfg\n" +
		"packagefile example.com/with=" + filepath.Join(withSidecar, "_pkg_.a") + "\n" +
		"packagefile example.com/without=" + filepath.Join(withoutSidecar, "_pkg_.a") + "\n" +
		"modinfo \"dummy\"\n"
	if err := os.WriteFile(importcfg, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readImportSummaries(importcfg)
	if err != nil {
		t.Fatalf("readImportSummaries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d (%v)", len(got), got)
	}
	if !reflect.DeepEqual(got["example.com/with"], wantSummary) {
		t.Errorf("mismatched summary for example.com/with")
	}
	if _, present := got["example.com/without"]; present {
		t.Errorf("no sidecar for example.com/without — should be absent from map")
	}
}

// TestAnalyze_CrossPackageDischarge: feed the analyzer an
// imports map carrying an imported callee's summary and verify
// that a pkg.Target(x) call with a prior pkg.IsPositive guard
// discharges, while an unguarded call does not.
func TestAnalyze_CrossPackageDischarge(t *testing.T) {
	// B's summary: Target requires isPositive on param 0.
	bSummary := &PackageSummary{
		ImportPath: "example.com/b",
		Funcs: map[string]*FuncSummary{
			"Target": {
				Name: "Target",
				ParamPreds: map[int][]Predicate{
					0: {{Pkg: "example.com/b", Name: "IsPositive"}},
				},
			},
		},
	}

	// A's source guards the call.
	okSrc := `package a

import "example.com/b"

func caller(x int) {
	if b.IsPositive(x) {
		b.Target(x)
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "in.go", okSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	aSum := &PackageSummary{
		ImportPath: "example.com/a",
		Funcs:      map[string]*FuncSummary{},
	}
	imports := map[string]*PackageSummary{"example.com/b": bSummary}
	imp := collectImports(f)
	var calls []CallDischarge
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			calls = append(calls, AnalyzeFunc(fn, aSum, imp, imports, fset, nil)...)
		}
	}
	// The call should be recorded as a discharge; CalleePkg
	// reflects the imported package.
	if len(calls) != 1 {
		t.Fatalf("want 1 call discharge, got %d: %v", len(calls), calls)
	}
	if calls[0].CalleePkg != "example.com/b" {
		t.Errorf("CalleePkg: got %q, want example.com/b", calls[0].CalleePkg)
	}
	for _, p := range calls[0].Params {
		if len(p.Missing) != 0 {
			t.Errorf("expected discharge, got missing %v", p.Missing)
		}
	}
}

func TestAnalyze_CrossPackageUnguardedLeavesMissing(t *testing.T) {
	bSummary := &PackageSummary{
		ImportPath: "example.com/b",
		Funcs: map[string]*FuncSummary{
			"Target": {
				Name: "Target",
				ParamPreds: map[int][]Predicate{
					0: {{Pkg: "example.com/b", Name: "IsPositive"}},
				},
			},
		},
	}

	src := `package a

import "example.com/b"

func caller(x int) {
	b.Target(x)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "in.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	aSum := &PackageSummary{
		ImportPath: "example.com/a",
		Funcs:      map[string]*FuncSummary{},
	}
	imports := map[string]*PackageSummary{"example.com/b": bSummary}
	imp := collectImports(f)
	var calls []CallDischarge
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			calls = append(calls, AnalyzeFunc(fn, aSum, imp, imports, fset, nil)...)
		}
	}
	if len(calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(calls))
	}
	c := calls[0]
	if !c.Undischarged() {
		t.Errorf("expected Undischarged, got params %v", c.Params)
	}
}
