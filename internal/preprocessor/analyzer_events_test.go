package preprocessor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestScopeTree_AncestorOrSelf covers the visibility primitive the
// Phase 3 resolver will use: a scope is visible to a query iff it is
// an ancestor or equal. Branch siblings must be invisible to each
// other; every enclosing ancestor must be visible.
func TestScopeTree_AncestorOrSelf(t *testing.T) {
	// Build:      root (0)
	//             /    \
	//          then(1)  else(2)
	//          /
	//       inner(3)
	s := newScopeTree()
	thenS := s.open(0)
	elseS := s.open(0)
	inner := s.open(thenS)

	cases := []struct {
		name string
		a, b int
		want bool
	}{
		{"self", 0, 0, true},
		{"root visible to then", 0, thenS, true},
		{"root visible to inner", 0, inner, true},
		{"then visible to inner", thenS, inner, true},
		{"then NOT visible to else (siblings)", thenS, elseS, false},
		{"inner NOT visible to then (descendant not ancestor)", inner, thenS, false},
		{"else NOT visible to inner (different branch)", elseS, inner, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.isAncestorOrSelf(c.a, c.b); got != c.want {
				t.Fatalf("isAncestorOrSelf(%d,%d) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestEventRecorder_Basic exercises the core recorder API: emissions
// are ordered, scope assignment follows enter/leave pairs, and the
// specialised drop rules (blank var, empty alts) silently skip.
func TestEventRecorder_Basic(t *testing.T) {
	r := newEventRecorder()
	pred := Predicate{Pkg: "p", Name: "Positive"}

	r.sourceLeaf(token.Pos(10), pred, "x")
	prev := r.enterScope()
	r.sourceLeaf(token.Pos(20), pred, "y")
	r.write(token.Pos(30), "y")
	r.leaveScope(prev)
	r.queryLeaf(token.Pos(40), pred, "x", 0, 0)

	if len(r.events) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(r.events))
	}

	if r.events[0].Scope != 0 {
		t.Fatalf("event 0 scope = %d, want root (0)", r.events[0].Scope)
	}
	if r.events[1].Scope == 0 {
		t.Fatalf("event 1 scope should be a non-root child, got %d", r.events[1].Scope)
	}
	if r.events[2].Scope != r.events[1].Scope {
		t.Fatalf("events 1 and 2 should share the inner scope, got %d vs %d",
			r.events[1].Scope, r.events[2].Scope)
	}
	if r.events[3].Scope != 0 {
		t.Fatalf("event 3 should be back in root scope, got %d", r.events[3].Scope)
	}
}

// TestEventRecorder_Drops covers the no-op paths: blank var / empty
// alts should produce no event, matching the FactSet's silent-drop
// behavior so the event stream and the FactSet stay in lockstep.
func TestEventRecorder_Drops(t *testing.T) {
	r := newEventRecorder()
	pred := Predicate{Pkg: "p", Name: "P"}

	r.sourceLeaf(token.Pos(1), pred, "") // blank var — dropped
	r.sourceOr(token.Pos(2), nil, "x")   // empty alts — dropped
	r.sourceOr(token.Pos(3), []Predicate{pred}, "") // blank var — dropped
	r.write(token.Pos(4), "")            // blank var — dropped

	if len(r.events) != 0 {
		t.Fatalf("expected all 4 calls to drop, got %d events", len(r.events))
	}
}

// TestEventStream_IfGuardProducesSourceAndQuery is an end-to-end
// integration check on the emission wiring: a tiny program with an
// if-guard preceding a call whose callee has a precondition should
// produce at least one leaf Source event inside a non-root scope
// (the then-branch) and one leaf Query event for the call's arg.
// Catches wholesale regressions in guard-fact emission or query
// emission without pinning exact event counts (which would be
// brittle against further wiring additions).
func TestEventStream_IfGuardProducesSourceAndQuery(t *testing.T) {
	const src = `package sample

func isPositive(x int) bool { return x > 0 }

func target(x int) {}

func caller(x int) {
	if isPositive(x) {
		target(x)
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	sum := &PackageSummary{
		ImportPath: "sample",
		Funcs:      make(map[string]*FuncSummary),
	}
	scanFile(sum, "sample", fset, f, nil)
	// The scanner only records funcs that declare obligations. Plant
	// one directly on target so caller's call to target(x) becomes a
	// query-emitting discharge check.
	pred := Predicate{Pkg: "sample", Name: "isPositive"}
	sum.Funcs["target"] = &FuncSummary{
		Name:       "target",
		ParamPreds: map[int][]Predicate{0: {pred}},
	}

	imp := collectImports(f)
	var callerDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "caller" {
			callerDecl = fn
			break
		}
	}
	if callerDecl == nil {
		t.Fatal("could not locate caller FuncDecl")
	}

	a := &analyzer{
		summary: sum,
		imp:     imp,
		rec:     newEventRecorder(),
	}
	a.analyzeBlock(callerDecl.Body)

	var sawGuardSource, sawCallQuery bool
	for _, ev := range a.rec.events {
		switch ev.Kind {
		case evSourceLeaf:
			if ev.Pred == pred && ev.Var == "x" && ev.Scope != 0 {
				sawGuardSource = true
			}
		case evQueryLeaf:
			if ev.Pred == pred && ev.Var == "x" {
				sawCallQuery = true
			}
		}
	}
	if !sawGuardSource {
		t.Errorf("expected a leaf Source in a non-root scope for isPositive on x (guard-fact emission)")
	}
	if !sawCallQuery {
		t.Errorf("expected a leaf Query for isPositive on x (target's parameter obligation)")
	}
}
