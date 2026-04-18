package preprocessor

import (
	"go/ast"
	"testing"
)

// fakeFunc constructs the minimal *ast.FuncDecl that
// recordDerivedReturnsIfChanged needs (just the name).
func fakeFunc(name string) *ast.FuncDecl {
	return &ast.FuncDecl{Name: &ast.Ident{Name: name}}
}

// TestRecordDerivedReturnsIfChanged_OrderIndependent locks down the
// equality semantics that the fixpoint loop in analyzePackageFiles
// depends on. intersectReturns iterates Go maps to build its leaf
// and Or-alt slices, so the same logical state can show up with
// different element orderings between iterations. If
// recordDerivedReturnsIfChanged reported every reorder as a change,
// the fixpoint would never converge and the safety cap would have
// to do all the work — papering over a bug that should be caught
// here instead.
func TestRecordDerivedReturnsIfChanged_OrderIndependent(t *testing.T) {
	pA := Predicate{Pkg: "pkg/p", Name: "A"}
	pB := Predicate{Pkg: "pkg/p", Name: "B"}
	pC := Predicate{Pkg: "pkg/p", Name: "C"}

	sum := &PackageSummary{Funcs: map[string]*FuncSummary{}}
	fn := fakeFunc("f")

	// First record: empty → non-empty. Must report change.
	if !recordDerivedReturnsIfChanged(sum, fn, []Predicate{pA, pB}, [][]Predicate{{pB, pC}}) {
		t.Fatalf("first non-empty record should report changed=true")
	}

	// Same content, different leaf order: should NOT report change.
	if recordDerivedReturnsIfChanged(sum, fn, []Predicate{pB, pA}, [][]Predicate{{pB, pC}}) {
		t.Fatalf("leaf reorder should NOT report changed=true")
	}

	// Same content, different Or-alt internal order: should NOT report change.
	if recordDerivedReturnsIfChanged(sum, fn, []Predicate{pA, pB}, [][]Predicate{{pC, pB}}) {
		t.Fatalf("Or-alt internal reorder should NOT report changed=true")
	}

	// Add an additional alt set: real change — must report.
	if !recordDerivedReturnsIfChanged(sum, fn, []Predicate{pA, pB}, [][]Predicate{{pB, pC}, {pA, pC}}) {
		t.Fatalf("adding an Or-alt set should report changed=true")
	}

	// Same final state but with the OUTER ordering of Or-alt sets
	// flipped (intersectReturns can produce either): should NOT
	// report change.
	if recordDerivedReturnsIfChanged(sum, fn, []Predicate{pA, pB}, [][]Predicate{{pA, pC}, {pB, pC}}) {
		t.Fatalf("outer Or-alt set reorder should NOT report changed=true")
	}
}

// TestRecordDerivedReturnsIfChanged_NoEntryForEmpty guards against
// pollution of sum.Funcs with empty entries for functions that have
// no derived returns. Empty-on-empty must be a no-op so we do not
// emit useless sidecar entries for un-annotated free functions.
func TestRecordDerivedReturnsIfChanged_NoEntryForEmpty(t *testing.T) {
	sum := &PackageSummary{Funcs: map[string]*FuncSummary{}}
	fn := fakeFunc("ghost")

	if recordDerivedReturnsIfChanged(sum, fn, nil, nil) {
		t.Fatalf("empty-on-empty record should report changed=false")
	}
	if _, ok := sum.Funcs["ghost"]; ok {
		t.Fatalf("empty-on-empty record should NOT create a Funcs entry")
	}
}
