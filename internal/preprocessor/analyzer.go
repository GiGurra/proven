package preprocessor

// Phase 3: flow-sensitive discharge (caller side).
//
// For each call in the caller's body to a function whose obligations
// Phase 2's scanner summarized, decide which obligations the
// caller's flow context already discharges and which remain
// missing. The decision is used later (Phase 5) to erase discharged
// proven.That / proven.Returns calls and diagnose undischarged ones.
//
// Scope intentionally narrow:
//
//   - Tracks facts only on direct identifier variables. Expressions
//     like `a.b` or `f(x)` do not participate as subjects.
//   - Same-package callees only. Cross-package obligations are Phase 6.
//   - Free functions and package-level variables holding the callee
//     (identified by *ast.Ident of call.Fun). Method calls need type
//     information to resolve the receiver type — deferred.
//   - Linear forward walk per block. When an if-statement has a
//     branch that always escapes (return or panic), the surviving
//     branch's facts persist to subsequent statements in the
//     enclosing block. No full dataflow merge across complex
//     control flow.
//
// The five fact sources listed in docs/todo/roadmap.md Phase 3 are
// all supported:
//
//   1. Preceding check: `if pred(x) { CALL }` — CALL sees fact(pred, x).
//   2. Early-return guard: `if !pred(x) { return }; CALL` — CALL
//      and subsequent stmts see fact(pred, x).
//   3. Conjoined guards: `if a(x) && b(y) { CALL }` — both facts
//      live inside the then-branch.
//   4. proven.Returns-annotated callee: `v := annotated()` — each
//      of annotated's ReturnPreds becomes a fact on v.
//   5. prove.That / prove.Must success: `v, err := prove.That(y, pred)`
//      followed by an `if err != nil { escape }` guard (or an `if err
//      == nil` block) produces fact(pred, v) in the success path.
//      prove.Must produces the fact unconditionally at its call site.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
)

// proveImportPath is the runtime-boundary-validation package
// whose successful calls the analyzer treats as fact sources.
const proveImportPath = "github.com/GiGurra/proven/pkg/prove"

// trustImportPath is the local-assertion package (pkg/trust).
// A trust.That call does not perform a runtime check; the
// analyzer treats it as an unconditional fact-injection on the
// assignment's LHS, the way prove.Must is treated but without
// the runtime side-effect.
const trustImportPath = "github.com/GiGurra/proven/pkg/trust"

// Fact asserts that predicate Pred holds on the variable named Var
// at a given program point.
type Fact struct {
	Pred Predicate
	Var  string
}

// FactSet is a compact intermediate used only by collectGuardFacts
// and the emit-as-sources helpers. It carries the (predicate, var)
// pairs (and disjunctive equivalents) that a guard expression proves
// when true, so they can be emitted as Source events at the branch
// boundary. The previous full-featured implementation (Has, Clone,
// Forget, etc.) was retired with the FactSet-based resolver in
// Phase 3; only the accumulator surface remains.
type FactSet struct {
	m   map[Fact]struct{}
	ors map[string]map[string][]Predicate
}

func newFactSet() *FactSet {
	return &FactSet{
		m:   make(map[Fact]struct{}),
		ors: make(map[string]map[string][]Predicate),
	}
}

// Add records a leaf fact in the set; duplicates are coalesced.
func (fs *FactSet) Add(f Fact) { fs.m[f] = struct{}{} }

// orKey builds a canonical comparison key for a set of Or-alts.
// Predicates are rendered as "pkg|name" and sorted so alt order is
// irrelevant to equality.
func orKey(alts []Predicate) string {
	parts := make([]string, len(alts))
	for i, p := range alts {
		parts[i] = p.Pkg + "|" + p.Name
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

// CallDischarge records the obligation-discharge status for one
// call site analyzed against the caller's flow facts.
//
// CalleePkg is empty for same-package callees and holds the
// callee's import path for cross-package ones; CalleeKey is the
// summary key within that package (bare function name, or
// "Recv.Method" for methods). Together they uniquely identify
// which callee's obligation signature was consulted.
type CallDischarge struct {
	Pos       token.Pos
	CalleePkg string
	CalleeKey string
	Params    []ParamDischarge
}

// Undischarged reports whether any parameter of this call site
// still has a missing predicate after flow analysis.
func (c CallDischarge) Undischarged() bool {
	for _, p := range c.Params {
		if len(p.Missing) > 0 || len(p.MissingOrs) > 0 {
			return true
		}
	}
	return false
}

// ParamDischarge records per-parameter discharge: which predicates
// the callee required (from Phase 2's summary) and which remained
// missing from the caller's fact set at the call site.
//
// RequiredOrs / MissingOrs carry the disjunctive-obligation side:
// each entry is one proven.Or(alts...) obligation on the parameter.
// An Or-obligation is discharged when any alt leaf holds or a
// matching Or-fact was planted. Surfaced separately from the leaf
// lists so diagnostics can render the whole Or at once instead of
// emitting one complaint per disjunct.
type ParamDischarge struct {
	ParamIdx    int
	ArgName     string
	Required    []Predicate
	Missing     []Predicate
	RequiredOrs [][]Predicate
	MissingOrs  [][]Predicate
}

// AnalyzeFunc walks caller's body and returns one CallDischarge
// per call to a function whose key appears in summary.Funcs
// (same-package) or in any of imports[pkg].Funcs (cross-package).
// Callees without a summary entry (external, or annotated only
// via proven.Returns with no ParamPreds) produce no entries.
//
// imports may be nil — same-package-only mode. Tests that do not
// need cross-package resolution pass nil; Phase 6's compile path
// assembles the map from the compile's -importcfg.
//
// fset + diags enable strict-mode reporting at prove/trust/proven
// sites where a predicate expression cannot be resolved to a named
// func or pkg.Name selector. Pass nil for either to disable strict
// mode (used by tests that do not surface diagnostics).
func AnalyzeFunc(caller *ast.FuncDecl, summary *PackageSummary, imp *importInfo, imports map[string]*PackageSummary, fset *token.FileSet, diags *[]Diagnostic) []CallDischarge {
	calls, _, _ := AnalyzeFuncWithReturns(caller, summary, imp, imports, fset, diags)
	return calls
}

// AnalyzeFuncWithReturns is the richer variant of AnalyzeFunc: in
// addition to the call-discharge list, it returns the derived
// postconditions — the intersection of leaf / Or facts on the
// returned identifier across every ReturnStmt. An empty slice is
// returned when the function has no return statements, returns a
// non-identifier result, or carries no facts at exit.
//
// The analyzer runs the same flow-sensitive walk either way; the
// return-snapshot bookkeeping is linear in the number of returns
// and does not add an asymptotic cost. Functions whose bodies
// establish no facts produce empty snapshots cheaply.
func AnalyzeFuncWithReturns(caller *ast.FuncDecl, summary *PackageSummary, imp *importInfo, imports map[string]*PackageSummary, fset *token.FileSet, diags *[]Diagnostic) ([]CallDischarge, []Predicate, [][]Predicate) {
	if caller.Body == nil {
		return nil, nil, nil
	}
	a := &analyzer{
		summary:     summary,
		imp:         imp,
		imports:     imports,
		rec:         newEventRecorder(),
		proveAlias:  imp.aliasFor(proveImportPath),
		trustAlias:  imp.aliasFor(trustImportPath),
		provenAlias: imp.provenAlias,
		fset:        fset,
		diags:       diags,
	}
	a.seedEventsFromPreconditions(caller, summary)
	a.analyzeBlock(caller.Body)
	leaves, ors := a.intersectReturns()
	return a.out, leaves, ors
}

// seedEventsFromPreconditions
// into the event stream: each parameter precondition of the caller
// itself is emitted as a source event at the function body's
// opening-brace position in the root scope. Keeps the stream's
// visibility semantics consistent — a body-level query can see
// precondition-seeded facts because they live in ancestor-or-self
// scope and precede the walk.
func (a *analyzer) seedEventsFromPreconditions(caller *ast.FuncDecl, summary *PackageSummary) {
	if summary == nil || caller.Body == nil {
		return
	}
	key := funcDeclKey(caller)
	fsum, ok := summary.Funcs[key]
	if !ok || fsum == nil {
		return
	}
	pos := caller.Body.Lbrace
	paramNames := paramNamesByIndex(caller.Type)
	for idx, preds := range fsum.ParamPreds {
		name, ok := paramNames[idx]
		if !ok || name == "" || name == "_" {
			continue
		}
		for _, p := range preds {
			a.rec.sourceLeaf(pos, p, name)
		}
	}
	for idx, ors := range fsum.ParamOrs {
		name, ok := paramNames[idx]
		if !ok || name == "" || name == "_" {
			continue
		}
		for _, alts := range ors {
			a.rec.sourceOr(pos, alts, name)
		}
	}
}

// funcDeclKey reproduces the FuncSummary.Key() computation starting
// from a FuncDecl: "Name" for free functions, "Recv.Method" for
// methods. Used to look up the function's own precondition record
// in summary.Funcs when seeding the analyzer's fact set.
func funcDeclKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := receiverTypeName(fn.Recv.List[0].Type)
	if recv == "" {
		return fn.Name.Name
	}
	return recv + "." + fn.Name.Name
}

// paramNamesByIndex maps each parameter position to its name (or
// the empty string for unnamed / blank-identifier parameters). The
// 0-based positions match the ones the scanner uses when populating
// FuncSummary.ParamPreds, so the fact seeder can pair them up.
func paramNamesByIndex(ft *ast.FuncType) map[int]string {
	out := map[int]string{}
	if ft == nil || ft.Params == nil {
		return out
	}
	pos := 0
	for _, field := range ft.Params.List {
		if len(field.Names) == 0 {
			pos++
			continue
		}
		for _, n := range field.Names {
			out[pos] = n.Name
			pos++
		}
	}
	return out
}

// analyzer owns the event-stream recorder, the read-only
// summary/import context (including any imported package summaries),
// and the growing list of discharge results.
//
// Phase 3 of the demand-driven restructure removed the parallel
// FactSet — all fact-mutation and fact-query paths now go through
// the recorded event stream and the on-demand backward resolver.
type analyzer struct {
	summary     *PackageSummary
	imp         *importInfo
	imports     map[string]*PackageSummary
	rec         *eventRecorder
	out         []CallDischarge
	proveAlias  string
	trustAlias  string
	provenAlias string
	fset        *token.FileSet
	diags       *[]Diagnostic

	// returnSnapshots collects one entry per ReturnStmt whose result
	// is a plain identifier, snapshotting the leaf / Or facts on that
	// identifier at the return point. Returns whose result is not an
	// identifier (literals, expressions, multi-value returns we do
	// not decompose) land as empty snapshots — they contribute no
	// predicates, which intersect-with-empty correctly collapses the
	// whole function's inferred postcondition to nothing. Unsound
	// otherwise: a function that sometimes returns a literal cannot
	// advertise facts that only hold on the non-literal branch.
	returnSnapshots []returnSnapshot
	sawReturn       bool
}

type returnSnapshot struct {
	leaves []Predicate
	ors    [][]Predicate
}

// analyzeBlock walks the statements of block in order under the
// analyzer's current fact set. Each statement may:
//   - consume the current facts (when a call is analyzed);
//   - introduce new facts for the remainder of the block (an
//     early-return guard or a postcondition-producing assignment);
//   - enter nested scopes with their own fact set (an if-body).
//
// Facts added inside this function outlive the block they were
// introduced in, which matches Go's lexical-scoping approximation
// for short-lived analyses. The block-scoping that does matter —
// facts introduced by an if-body — is handled inside analyzeIf
// where the fact set is saved and restored.
//
// One pattern must be recognized across two consecutive
// statements: the prove.That boundary idiom
//
//	v, err := prove.That(y, pred...)
//	if err != nil { return ... }
//
// The fact that `pred` holds on v is only established once the
// err-check guard has cleared the error path. Seeing the
// assignment alone is not enough; tryProvePattern pairs the two
// statements and plants facts in the right place.
func (a *analyzer) analyzeBlock(block *ast.BlockStmt) {
	i := 0
	for i < len(block.List) {
		if i+1 < len(block.List) && a.tryProvePattern(block.List[i], block.List[i+1]) {
			i += 2
			continue
		}
		a.analyzeStmt(block.List[i])
		i++
	}
}

func (a *analyzer) analyzeStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		a.analyzeIf(s)
	case *ast.ExprStmt:
		a.walkCalls(s.X)
		// A call like `f(&x)` or `f(x)` on a pointer may mutate x —
		// we cannot tell without the callee's body. Invalidate any
		// ident whose address escapes into this expression.
		a.invalidateAddrEscape(s.X)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			a.walkCalls(rhs)
		}
		// Reassignment invalidates: whatever the LHS identifier was
		// bound to, it's bound to something new now.
		for _, lhs := range s.Lhs {
			a.forgetLHS(lhs)
		}
		a.applyAssignPostconditions(s)
		// An `&x` anywhere in the RHS means x's address escaped —
		// the new alias can mutate x invisibly to the analyzer.
		for _, rhs := range s.Rhs {
			a.invalidateAddrEscape(rhs)
		}
	case *ast.IncDecStmt:
		if id, ok := s.X.(*ast.Ident); ok && id.Name != "_" {
			a.rec.write(id.Pos(), id.Name)
		}
	case *ast.DeclStmt:
		if gen, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, v := range vs.Values {
						a.walkCalls(v)
					}
					a.applyDeclPostconditions(vs)
				}
			}
		}
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			a.walkCalls(r)
		}
		a.snapshotReturn(s)
	case *ast.BlockStmt:
		prevScope := a.rec.enterScope()
		a.analyzeBlock(s)
		a.rec.leaveScope(prevScope)
	case *ast.ForStmt:
		prevScope := a.rec.enterScope()
		if s.Body != nil {
			a.analyzeBlock(s.Body)
		}
		a.rec.leaveScope(prevScope)
	case *ast.RangeStmt:
		prevScope := a.rec.enterScope()
		if s.Body != nil {
			a.analyzeBlock(s.Body)
		}
		a.rec.leaveScope(prevScope)
	case *ast.SwitchStmt:
		if s.Body != nil {
			// Each case clause is its own branch sibling — open a
			// fresh scope per clause so facts established inside
			// one case do not leak into the next.
			for _, cc := range s.Body.List {
				if cl, ok := cc.(*ast.CaseClause); ok {
					prevScope := a.rec.enterScope()
					for _, cs := range cl.Body {
						a.analyzeStmt(cs)
					}
					a.rec.leaveScope(prevScope)
				}
			}
		}
	}
}

// forgetLHS drops facts on whatever identifier the assignment LHS
// is about to clobber. Supported shapes:
//
//   - bare ident (`x = ...`, `x := ...`)
//   - selector chain (`x.a.b = ...`) — forget the chain root x,
//     since the mutation changes x's field state and any predicate
//     that inspects x's fields may no longer hold
//   - index (`x[i] = ...`) — forget x
//   - `_` — no-op
//
// Dereference-assign (`*p = ...`) is NOT traced: without alias
// tracking we do not know which variable p points at. This is a
// documented gap — see the README "mutation and soundness" note.
func (a *analyzer) forgetLHS(lhs ast.Expr) {
	switch e := lhs.(type) {
	case *ast.Ident:
		if e.Name != "_" {
			a.rec.write(e.Pos(), e.Name)
		}
	case *ast.SelectorExpr:
		if root := selectorChainRoot(e); root != "" {
			a.rec.write(e.Pos(), root)
		}
	case *ast.IndexExpr:
		if id, ok := e.X.(*ast.Ident); ok && id.Name != "_" {
			a.rec.write(e.Pos(), id.Name)
		}
	}
}

// selectorChainRoot walks down `x.a.b.c` to the leftmost identifier
// and returns its name; returns "" for selector chains that do not
// bottom out in an ident (e.g. `pkg.Foo.Bar` where pkg is an import
// alias — the root is not a local variable to forget).
func selectorChainRoot(e *ast.SelectorExpr) string {
	switch x := e.X.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return selectorChainRoot(x)
	}
	return ""
}

// invalidateAddrEscape walks expr (but not down into nested block
// bodies) looking for `&ident` subexpressions. For each one, it
// forgets the ident's facts — the address has escaped, so any
// pointer-holding alias may mutate the value invisibly to the
// analyzer from this point on.
//
// The blocks filter keeps if/for-body statements that contain
// their own `&x` usage from triggering forgets on the outer scope.
// Those inner scopes are already handled by the clone-and-restore
// wrappers around analyzeBlock.
func (a *analyzer) invalidateAddrEscape(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		if _, isBlock := n.(*ast.BlockStmt); isBlock {
			return false
		}
		u, ok := n.(*ast.UnaryExpr)
		if !ok || u.Op != token.AND {
			return true
		}
		if id, ok := u.X.(*ast.Ident); ok && id.Name != "_" {
			a.rec.write(u.Pos(), id.Name)
		}
		return true
	})
}

// analyzeIf handles the two if-statement patterns that establish
// facts:
//
//  1. `if guard { body }` — facts implied by guard hold inside
//     body. On exit, if body always escapes, the negation of
//     guard's facts persists to subsequent statements.
//  2. `if guard { body } else { elseBody }` — guard-facts hold
//     inside body, their negation inside elseBody. If exactly
//     one branch escapes, the other's surviving facts persist.
//
// The escape check recognizes ReturnStmt and calls to panic as
// unconditional exits; anything else is treated as fall-through
// for conservatism (false negatives are safe here — they just
// reduce what the analyzer can discharge, never what it accepts).
func (a *analyzer) analyzeIf(ifStmt *ast.IfStmt) {
	guardFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, false, guardFacts)
	negFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, true, negFacts)

	// then-branch opens its own scope; guard facts live inside it.
	thenScope := a.rec.enterScope()
	a.emitFactSetAsSources(guardFacts, ifStmt.Cond.Pos())
	a.analyzeBlock(ifStmt.Body)
	a.rec.leaveScope(thenScope)
	thenEscapes := blockAlwaysEscapes(ifStmt.Body)

	// Optional else-branch: negated guard facts live in a sibling
	// scope to the then-branch, so the two do not cross-pollinate.
	elseEscapes := false
	if ifStmt.Else != nil {
		elseScope := a.rec.enterScope()
		a.emitFactSetAsSources(negFacts, ifStmt.Cond.Pos())
		switch e := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			a.analyzeBlock(e)
			elseEscapes = blockAlwaysEscapes(e)
		case *ast.IfStmt:
			a.analyzeIf(e)
			// Nested if-else chains: conservatively treat as
			// non-escaping so later facts are not over-claimed.
		}
		a.rec.leaveScope(elseScope)
	}

	// Post-if join: when exactly one branch always escapes, the
	// opposite branch's guard-facts "survive" into the enclosing
	// scope as if the escaping branch had never been taken. Emit
	// them at the if-statement's closing position in the current
	// (enclosing) scope so follow-on code sees them.
	postPos := ifStmt.End()
	switch {
	case thenEscapes && !elseEscapes:
		a.emitFactSetAsSources(negFacts, postPos)
	case !thenEscapes && elseEscapes:
		a.emitFactSetAsSources(guardFacts, postPos)
	}
}

// emitFactSetAsSources mirrors the leaves / Or-facts inside fs as
// source events at pos in the current scope. Called at each site
// where the FactSet-based analyzer would union a derived FactSet
// (guard facts, negated-guard facts, post-if-join facts) into the
// live state.
func (a *analyzer) emitFactSetAsSources(fs *FactSet, pos token.Pos) {
	for f := range fs.m {
		a.rec.sourceLeaf(pos, f.Pred, f.Var)
	}
	for v, m := range fs.ors {
		for _, alts := range m {
			a.rec.sourceOr(pos, alts, v)
		}
	}
}

// collectGuardFacts walks a boolean expression used as an
// if-condition and adds to out each (predicate, variable) pair
// the expression proves when it evaluates true (negated=false) or
// false (negated=true).
//
// Supported shapes:
//
//   - `pred(x)`                          — fact(pred, x) on true
//   - `!pred(x)`                         — fact(pred, x) on false
//   - `pkg.Pred(x)`                      — same, cross-package
//   - `a && b`                           — recurse on both with same polarity
//   - `!(a && b)` is not distributed; only outer-most ! on a simple
//     predicate call is recognized, matching the fixture shapes in
//     docs/todo/roadmap.md Phase 3. `||` is ignored for the same
//     reason — a disjunction produces no positive fact.
func collectGuardFacts(expr ast.Expr, imp *importInfo, importPath string, negated bool, out *FactSet) {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			collectGuardFacts(e.X, imp, importPath, !negated, out)
		}
	case *ast.BinaryExpr:
		if e.Op == token.LAND && !negated {
			collectGuardFacts(e.X, imp, importPath, false, out)
			collectGuardFacts(e.Y, imp, importPath, false, out)
			return
		}
		// `x != nil` / `x == nil` plants the library NonNil / Nil
		// predicate as a flow fact on x, so downstream callees that
		// require proven.NonNil (or Nil) are satisfied without a
		// redundant predicate call in the guard.
		if e.Op == token.NEQ || e.Op == token.EQL {
			if v, ok := nilCompareVar(e, imp); ok {
				isNonNil := (e.Op == token.NEQ) != negated
				name := "Nil"
				if isNonNil {
					name = "NonNil"
				}
				out.Add(Fact{Pred: Predicate{Pkg: provenImportPath, Name: name}, Var: v})
			}
		}
		// Conservatively ignore OR and negated-AND.
	case *ast.ParenExpr:
		collectGuardFacts(e.X, imp, importPath, negated, out)
	case *ast.CallExpr:
		// Only produce a fact when the polarity matches: a bare
		// predicate call proves its truth when the condition is
		// true; the negated (`!pred(x)`) form proves it when the
		// condition is false.
		if !negated {
			if pred, v, ok := callAsPredicate(e, imp, importPath); ok {
				out.Add(Fact{Pred: pred, Var: v})
			}
		}
	}
}

// nilCompareVar reports the canonical-key subject of a `x != nil`
// or `x == nil` BinaryExpr, accepting either order (`nil != x` works
// too) and either identifier or selector-chain subjects. Returns
// (_, false) when neither side is the bare `nil` identifier or when
// the other side does not canonicalize to a tracked expression key.
func nilCompareVar(e *ast.BinaryExpr, imp *importInfo) (string, bool) {
	isNilIdent := func(expr ast.Expr) bool {
		id, ok := expr.(*ast.Ident)
		return ok && id.Name == "nil"
	}
	switch {
	case isNilIdent(e.Y):
		return exprKey(e.X, imp)
	case isNilIdent(e.X):
		return exprKey(e.Y, imp)
	}
	return "", false
}

// callAsPredicate matches a call expression of the form
// `predIdent(x)` or `pkgAlias.Pred(x)` where x canonicalizes to a
// tracked expression key (identifier or selector chain), and returns
// the resolved predicate identity plus the subject key.
func callAsPredicate(call *ast.CallExpr, imp *importInfo, importPath string) (Predicate, string, bool) {
	if len(call.Args) != 1 {
		return Predicate{}, "", false
	}
	subject, ok := exprKey(call.Args[0], imp)
	if !ok {
		return Predicate{}, "", false
	}
	pred, ok := resolvePredicate(call.Fun, imp, importPath)
	if !ok {
		return Predicate{}, "", false
	}
	return pred, subject, true
}

// blockAlwaysEscapes reports whether a block's control flow
// cannot fall out the bottom — i.e. whether the statement after
// the enclosing if-stmt will never run after this branch
// executes. Recognizes: a return statement anywhere at the
// top level, and a call to panic as a top-level expression.
//
// Deliberately simple. Under-approximating (returning false for a
// block that in fact always escapes) loses facts but never invents
// them, which is the safe direction.
func blockAlwaysEscapes(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.ExprStmt:
			if call, ok := s.X.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
					return true
				}
			}
		}
	}
	return false
}

// walkCalls descends into expr looking for call expressions that
// match summary entries, and records their discharge against the
// current fact set. Walks nested calls (e.g. `outer(annotated(x))`)
// so obligations on inner calls are checked. Also verifies each
// proven.Returns call in the subtree: the value argument must be
// an identifier with every listed predicate already established in
// the current fact set. Unverified postconditions are a soundness
// hole (callers get a wrong fact) and emit a strict-mode diagnostic.
func (a *analyzer) walkCalls(expr ast.Expr) {
	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			a.recordCallDischarge(call)
			a.verifyProvenReturns(call)
			a.plantFromInlineFactCall(call)
		}
		return true
	})
}

// plantFromInlineFactCall plants sources for prove.Must / trust.That
// calls when the first argument is a canonically-keyable expression.
// This makes the in-place idiom
//
//	prove.Must(holder.Value, proven.NonNil)
//	needsNonNil(holder.Value)
//
// discharge without requiring a local binding: once prove.Must has
// returned without panicking, the predicate holds on its first
// argument, irrespective of whether the return value was captured.
// Same reasoning for trust.That (the programmer vouches for the
// predicate at the call site).
//
// Assigned forms (`v := prove.Must(x, p)`) still plant on the LHS in
// applyAssignPostconditions; this function plants on the first-arg
// key additionally, since the two keys may differ (the LHS has a new
// identity; the first-arg key is whatever the caller passed in).
//
// prove.That is intentionally omitted: its bare form discards the
// (value, error) pair and therefore tells the analyzer nothing —
// whether the predicate held at runtime is unknowable from the call
// site's AST without the err-check pattern tryProvePattern handles.
func (a *analyzer) plantFromInlineFactCall(call *ast.CallExpr) {
	if len(call.Args) < 2 {
		return
	}
	var preds TrailingPreds
	switch {
	case a.isProveCall(call, "Must"):
		preds = a.proveCallPredicates(call, "Must")
	case a.isTrustCall(call, "That"):
		preds = a.trustCallPredicates(call)
	default:
		return
	}
	if len(preds.Leaves) == 0 && len(preds.Ors) == 0 {
		return
	}
	key, ok := exprKey(call.Args[0], a.imp)
	if !ok {
		return
	}
	a.plantTrailingFacts(preds, key, call.Args[0].Pos())
}

// verifyProvenReturns checks that every predicate supplied to
// proven.Returns is already a fact on the value argument in the
// analyzer's current flow state. This is the verification step for
// return-value postconditions: the scanner advertises the
// predicates on the containing function's summary, but without this
// check the claim would be accepted on the programmer's word rather
// than proved. Violations emit a Go-standard diagnostic via the
// analyzer's diags channel.
//
// The value argument must be a direct identifier; literals and
// composite expressions have no fact-set identity, so we refuse
// them explicitly. Users who want to return a literal as a
// postcondition-bearing value should wrap with prove.Must (runtime
// check) or trust.That (programmer vouch) first.
func (a *analyzer) verifyProvenReturns(call *ast.CallExpr) {
	if a.provenAlias == "" {
		return
	}
	if !isSel(call, a.provenAlias, "Returns") {
		return
	}
	if len(call.Args) < 2 {
		return
	}
	// Test files (foo_test.go) legitimately need to exercise
	// proven.Returns with deliberately-bad inputs to verify
	// runtime-check wiring through proventest.AssertFails and
	// friends. Strict site verification in these files would make
	// such wiring tests impossible to write. The rest of strict
	// mode (unresolvable-predicate rejection, etc.) still applies
	// in test files — that is about identity shape, not runtime
	// values.
	if a.fset != nil {
		if pos := a.fset.Position(call.Pos()); isTestFile(pos.Filename) {
			return
		}
	}
	valueID, ok := call.Args[0].(*ast.Ident)
	if !ok {
		reportBadReturnsValue(a.diags, a.fset, call.Args[0], "literal or expression")
		return
	}
	if a.diags == nil || a.fset == nil {
		// Lenient mode (stand-alone tests, no diagnostic sink);
		// skip verification so existing test helpers keep working.
		return
	}
	for _, arg := range call.Args[1:] {
		// Pass nil diags — the scanner's recordReturns pass has
		// already reported any unresolvable predicate, and we do
		// not want to double the error. resolveAndFlat here is
		// purely for the leaf / disjunction expansion so we can
		// verify each piece on the returned value.
		leaves, ors, ok := resolveAndFlat(arg, a.imp, a.summary.ImportPath, nil, nil, "")
		if !ok {
			continue
		}
		for _, pred := range leaves {
			// Emit a query event before checking so the resolver
			// can anchor its backward walk to the current scope.
			a.rec.queryLeaf(call.Pos(), pred, valueID.Name, -1, -1)
			if !a.discharged(pred, valueID.Name) {
				reportUnprovenReturns(a.diags, a.fset, call, pred, valueID.Name, a.summary.ImportPath)
			}
		}
		for _, alts := range ors {
			a.rec.queryOr(call.Pos(), alts, valueID.Name, -1, -1)
			if !a.dischargedOr(alts, valueID.Name) {
				reportUnprovenReturnsOr(a.diags, a.fset, call, alts, valueID.Name, a.summary.ImportPath)
			}
		}
	}
}

// isTestFile reports whether filename is a Go test file, identified
// by the `_test.go` suffix Go's own toolchain uses. Path prefix is
// irrelevant — we inspect only the base name.
func isTestFile(filename string) bool {
	if filename == "" {
		return false
	}
	base := filename
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '/' || base[i] == '\\' {
			base = base[i+1:]
			break
		}
	}
	const suffix = "_test.go"
	return len(base) >= len(suffix) && base[len(base)-len(suffix):] == suffix
}

// reportBadReturnsValue emits a diagnostic when the value argument
// of proven.Returns is not a direct identifier. This is a strict-
// mode rejection — the analyzer needs a name to look up facts in
// the current fact set, and literals / expressions have no such
// identity.
func reportBadReturnsValue(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, kind string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: value argument to proven.Returns must be an identifier (got %s); wrap with prove.Must or trust.That to establish the fact first",
			kind,
		),
	})
}

// reportUnprovenReturns emits a diagnostic when a predicate listed
// in proven.Returns has not been established on the value at the
// return site. This is the verification step that closes the
// soundness hole where proven.Returns would advertise a
// postcondition to callers without having proved it locally.
func reportUnprovenReturns(diags *[]Diagnostic, fset *token.FileSet, call *ast.CallExpr, pred Predicate, varName, currentPkg string) {
	pos := fset.Position(call.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: proven.Returns predicate %s is not established on %s — add a guard (if %s(%s) { ... }), prove.Must, trust.That, or another discharge path before returning",
			predicateLabelFor(pred, currentPkg), varName, predicateLabelFor(pred, currentPkg), varName,
		),
	})
}

// reportUnprovenReturnsOr emits a diagnostic when a proven.Or(...)
// disjunction listed in proven.Returns cannot be satisfied on the
// return value. The fix is the same as for unproven leaf returns
// (establish a fact via guard / prove / trust), but the message
// renders the whole Or so the user sees which alternatives are
// available.
func reportUnprovenReturnsOr(diags *[]Diagnostic, fset *token.FileSet, call *ast.CallExpr, alts []Predicate, varName, currentPkg string) {
	pos := fset.Position(call.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: proven.Returns disjunction proven.Or(%s) is not established on %s — satisfy at least one alternative (guard, prove, or trust) before returning",
			altsLabel(alts, currentPkg), varName,
		),
	})
}

// altsLabel renders an Or alt list for diagnostics: comma-separated
// predicate labels using predicateLabelFor for each.
func altsLabel(alts []Predicate, currentPkg string) string {
	parts := make([]string, len(alts))
	for i, p := range alts {
		parts[i] = predicateLabelFor(p, currentPkg)
	}
	return strings.Join(parts, ", ")
}

// predicateLabelFor renders a predicate for a diagnostic: same-
// package predicates use the bare name; cross-package ones keep a
// short package qualifier (last path segment).
func predicateLabelFor(p Predicate, currentPkg string) string {
	if p.Pkg == "" || p.Pkg == currentPkg {
		return p.Name
	}
	return lastPathSegment(p.Pkg) + "." + p.Name
}

// recordCallDischarge looks up the callee's summary (if any) and
// produces a CallDischarge for the call site. Callees whose key
// is not in the corresponding summary's Funcs produce no entry:
// either they are not annotated (nothing to discharge), or they
// are from an external package whose summary is not in
// a.imports (typical for stdlib and third-party code the
// preprocessor has not scanned).
func (a *analyzer) recordCallDischarge(call *ast.CallExpr) {
	calleePkg, key, ok := a.resolveCallee(call)
	if !ok {
		return
	}
	sum, ok := a.lookupCalleeSummary(calleePkg, key)
	if !ok {
		return
	}
	indices := paramIndicesUnion(sum)
	// dischargeIdx is the zero-based index this call will occupy in
	// a.out once appended below; used to stitch the associated query
	// events back to the CallDischarge record in later phases.
	dischargeIdx := len(a.out)
	var params []ParamDischarge
	for _, idx := range indices {
		if idx >= len(call.Args) {
			continue
		}
		required := sum.ParamPreds[idx]
		requiredOrs := sum.ParamOrs[idx]
		argName, restore := a.bindArgForCheck(call.Args[idx], idx, required, requiredOrs)

		var missing []Predicate
		for _, p := range required {
			a.rec.queryLeaf(call.Args[idx].Pos(), p, argName, dischargeIdx, idx)
			if !a.discharged(p, argName) {
				missing = append(missing, p)
			}
		}
		var missingOrs [][]Predicate
		for _, alts := range requiredOrs {
			a.rec.queryOr(call.Args[idx].Pos(), alts, argName, dischargeIdx, idx)
			if !a.dischargedOr(alts, argName) {
				missingOrs = append(missingOrs, append([]Predicate(nil), alts...))
			}
		}
		restore()

		pd := ParamDischarge{
			ParamIdx: idx,
			ArgName:  argName,
			Missing:  missing,
		}
		if len(required) > 0 {
			pd.Required = append([]Predicate(nil), required...)
		}
		if len(requiredOrs) > 0 {
			pd.RequiredOrs = make([][]Predicate, len(requiredOrs))
			for i, alts := range requiredOrs {
				pd.RequiredOrs[i] = append([]Predicate(nil), alts...)
			}
			pd.MissingOrs = missingOrs
		}
		params = append(params, pd)
	}
	if len(params) == 0 {
		return
	}
	a.out = append(a.out, CallDischarge{
		Pos:       call.Pos(),
		CalleePkg: calleePkg,
		CalleeKey: key,
		Params:    params,
	})
}

// paramIndicesUnion returns every parameter index that carries either
// a leaf or Or obligation in sum, sorted. Deterministic iteration
// order so diagnostics stay stable across runs.
func paramIndicesUnion(sum *FuncSummary) []int {
	seen := make(map[int]struct{}, len(sum.ParamPreds)+len(sum.ParamOrs))
	for idx := range sum.ParamPreds {
		seen[idx] = struct{}{}
	}
	for idx := range sum.ParamOrs {
		seen[idx] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	slices.Sort(out)
	return out
}

// bindArgForCheck returns the variable name to use when checking
// discharge for the idx-th argument, plus a `restore` closure that
// reverses any facts the analyzer temporarily plants.
//
// Four cases, tried in order:
//
//   - arg is a plain identifier: use it as-is, no virtual plant.
//   - arg is a nested *ast.CallExpr whose callee has postconditions
//     (explicit or derived): clone the fact set, plant the callee's
//     returnFacts on a synthetic variable name, and use that name.
//   - arg is a literal / simple compile-time expression AND one or
//     more of the callee's required predicates is library-known
//     (see litconst.go): evaluate each, plant a virtual fact for
//     every predicate that holds on the literal, and use the
//     synthetic variable for the discharge check. This is what lets
//     target(5) satisfy proven.That(x, proven.Positive) at build
//     time with no guard.
//   - anything else: return an empty argName and let the discharge
//     check report every predicate as missing.
//
// The restore closure puts the original fact set back after the
// per-argument check so virtual facts do not leak into subsequent
// statements. Only one level of nesting is examined per argument;
// deeper chains rely on the analyzer's own cascading recordCallDischarge.
func (a *analyzer) bindArgForCheck(arg ast.Expr, idx int, required []Predicate, requiredOrs [][]Predicate) (string, func()) {
	if key, ok := exprKey(arg, a.imp); ok {
		return key, noRestore
	}
	if inner, ok := arg.(*ast.CallExpr); ok {
		if pkg, key, ok := a.resolveCallee(inner); ok {
			if sum, ok := a.lookupCalleeSummary(pkg, key); ok {
				leaves, ors := returnFacts(sum)
				if len(leaves) > 0 || len(ors) > 0 {
					return a.plantVirtual(idx, leaves, ors)
				}
			}
		}
	}
	// Literal evaluation against library-known predicates.
	litLeaves := a.evalLiterals(arg, required, requiredOrs)
	if len(litLeaves) > 0 {
		return a.plantVirtual(idx, litLeaves, nil)
	}
	return "", noRestore
}

// evalLiterals runs each required predicate through the literal
// evaluator and returns the subset that hold on arg. Or-obligations
// are satisfied when any alt holds — we return that alt as a leaf
// so the existing dischargedOr path picks it up via the any-leaf
// rule. Predicates whose evaluator reports EvalSkip are ignored;
// their discharge falls back to the normal paths.
func (a *analyzer) evalLiterals(arg ast.Expr, required []Predicate, requiredOrs [][]Predicate) []Predicate {
	var out []Predicate
	for _, p := range required {
		if evalLibraryPredicate(p, arg) == EvalHolds {
			out = append(out, p)
		}
	}
	for _, alts := range requiredOrs {
		for _, p := range alts {
			if evalLibraryPredicate(p, arg) == EvalHolds {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// plantVirtual clones the fact set, opens a throw-away scope, adds
// leaves and ors on a synthetic `$argN` variable, and returns the
// name plus a restore closure that puts the original fact set and
// scope back.
//
// The throw-away scope is the key mechanism for keeping virtual
// facts from leaking into later queries: the queries generated for
// this parameter check are emitted inside the throw-away scope and
// see the virtual sources as ancestors; later queries (for the next
// parameter, or after the call) are emitted in an outer scope that
// is a non-descendant of the throw-away scope, so the virtual
// sources are invisible to them.
func (a *analyzer) plantVirtual(idx int, leaves []Predicate, ors [][]Predicate) (string, func()) {
	vname := fmt.Sprintf("$arg%d", idx)
	prevScope := a.rec.enterScope()
	pos := token.NoPos
	for _, p := range leaves {
		a.rec.sourceLeaf(pos, p, vname)
	}
	for _, alts := range ors {
		a.rec.sourceOr(pos, alts, vname)
	}
	return vname, func() {
		a.rec.leaveScope(prevScope)
	}
}

func noRestore() {}

// resolveCallee returns (calleePkg, summaryKey, ok) for a call
// whose callee the analyzer recognizes:
//
//   - bare identifier `Foo(...)` — same-package free function or
//     a package-level var bound to one; calleePkg == "".
//   - selector `pkg.Foo(...)` where pkg is a known import alias —
//     cross-package call; calleePkg is the import's path.
//
// Method calls on a value receiver are out of scope without type
// information and return (_, _, false).
func (a *analyzer) resolveCallee(call *ast.CallExpr) (string, string, bool) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return "", fn.Name, true
	case *ast.SelectorExpr:
		x, ok := fn.X.(*ast.Ident)
		if !ok {
			return "", "", false
		}
		if path, isImport := a.imp.aliases[x.Name]; isImport {
			return path, fn.Sel.Name, true
		}
	}
	return "", "", false
}

// lookupCalleeSummary returns the FuncSummary for a resolved
// callee. Same-package callees (calleePkg "" or matching the
// current package) look up in a.summary; cross-package ones look
// up in a.imports[calleePkg]. Missing entries yield (nil, false),
// meaning "no recorded obligations" — either the callee has none
// or we never saw its summary at all.
func (a *analyzer) lookupCalleeSummary(calleePkg, key string) (*FuncSummary, bool) {
	if calleePkg == "" || calleePkg == a.summary.ImportPath {
		sum, ok := a.summary.Funcs[key]
		return sum, ok
	}
	if imp, ok := a.imports[calleePkg]; ok {
		sum, ok := imp.Funcs[key]
		return sum, ok
	}
	return nil, false
}

// discharged reports whether predicate pred holds on the
// identifier named varName at the current emission point, either
// as a direct fact or by implication through the declared inference
// rules. Delegates to the event-stream resolver introduced in Phase
// 3; the call is kept as a tiny shim because most of the analyzer's
// internal paths still consult it by name.
//
// Unknown / not-in-scope variable names ("" from non-ident
// arguments) never discharge anything.
func (a *analyzer) discharged(pred Predicate, varName string) bool {
	return a.dischargedViaEvents(pred, varName)
}

// snapshotReturn records the fact set on a return's first result.
//
// If the result is an identifier, collect every leaf fact and every
// Or-fact visible on that identifier at the return point, using the
// event-stream resolver's backward walk. Otherwise (literal, multi-
// value return, expression) record an empty snapshot — the sound
// intersection rule will collapse the function's derived
// postconditions to nothing across a function that sometimes
// returns a literal. A return with no results (e.g. a bare `return`
// in a void function, or named-return shortcuts) contributes an
// empty snapshot too.
func (a *analyzer) snapshotReturn(s *ast.ReturnStmt) {
	a.sawReturn = true
	if len(s.Results) == 0 {
		a.returnSnapshots = append(a.returnSnapshots, returnSnapshot{})
		return
	}
	id, ok := s.Results[0].(*ast.Ident)
	if !ok {
		a.returnSnapshots = append(a.returnSnapshots, returnSnapshot{})
		return
	}
	leaves, ors := a.snapshotFactsOnVarViaEvents(id.Name)
	a.returnSnapshots = append(a.returnSnapshots, returnSnapshot{
		leaves: leaves,
		ors:    ors,
	})
}

// intersectReturns returns the leaf-predicate and Or-alt
// intersections across every recorded return snapshot. A function
// with no return statements at all (infinite-loop / os.Exit-only
// body) has sawReturn == false and returns (nil, nil) — callers
// should not advertise anything.
func (a *analyzer) intersectReturns() ([]Predicate, [][]Predicate) {
	if !a.sawReturn || len(a.returnSnapshots) == 0 {
		return nil, nil
	}
	// Leaf intersection via count.
	counts := make(map[Predicate]int)
	for _, snap := range a.returnSnapshots {
		seen := make(map[Predicate]struct{}, len(snap.leaves))
		for _, p := range snap.leaves {
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			counts[p]++
		}
	}
	var leaves []Predicate
	target := len(a.returnSnapshots)
	for p, c := range counts {
		if c == target {
			leaves = append(leaves, p)
		}
	}
	// Or-alt intersection via canonical-key count. Preserve the
	// first-seen alt ordering for readability in diagnostics.
	orCounts := make(map[string]int)
	orAlts := make(map[string][]Predicate)
	for _, snap := range a.returnSnapshots {
		seen := make(map[string]struct{}, len(snap.ors))
		for _, alts := range snap.ors {
			k := orKey(alts)
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			orCounts[k]++
			if _, recorded := orAlts[k]; !recorded {
				orAlts[k] = append([]Predicate(nil), alts...)
			}
		}
	}
	var ors [][]Predicate
	for k, c := range orCounts {
		if c == target {
			ors = append(ors, orAlts[k])
		}
	}
	return leaves, ors
}

// returnFacts reports the postconditions a callee advertises to its
// callers — the union of the explicit proven.Returns / trust.Returns
// lists (ReturnPreds / ReturnOrs) and the analyzer-derived intersection
// of fact sets at each ReturnStmt (DerivedReturnPreds /
// DerivedReturnOrs). Explicit claims are enforced by the strict-mode
// verifyProvenReturns pass; derived claims are sound by construction
// — only facts the analyzer itself established on the returned
// identifier at every return.
func returnFacts(sum *FuncSummary) ([]Predicate, [][]Predicate) {
	if len(sum.DerivedReturnPreds) == 0 && len(sum.DerivedReturnOrs) == 0 {
		return sum.ReturnPreds, sum.ReturnOrs
	}
	leaves := append([]Predicate(nil), sum.ReturnPreds...)
	seen := make(map[Predicate]struct{}, len(leaves))
	for _, p := range leaves {
		seen[p] = struct{}{}
	}
	for _, p := range sum.DerivedReturnPreds {
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		leaves = append(leaves, p)
	}
	ors := append([][]Predicate(nil), sum.ReturnOrs...)
	seenOrs := make(map[string]struct{}, len(ors))
	for _, alts := range ors {
		seenOrs[orKey(alts)] = struct{}{}
	}
	for _, alts := range sum.DerivedReturnOrs {
		k := orKey(alts)
		if _, dup := seenOrs[k]; dup {
			continue
		}
		seenOrs[k] = struct{}{}
		ors = append(ors, alts)
	}
	return leaves, ors
}

// plantTrailingFacts adds every leaf and disjunctive fact carried
// by tp as a fact on variable v. Shared between the prove.Must /
// trust.That assignment sites so the Or-handling stays in one place.
// pos is the source position used when emitting the parallel event
// stream entries.
func (a *analyzer) plantTrailingFacts(tp TrailingPreds, v string, pos token.Pos) {
	for _, p := range tp.Leaves {
		a.rec.sourceLeaf(pos, p, v)
	}
	for _, alts := range tp.Ors {
		a.rec.sourceOr(pos, alts, v)
	}
}

// dischargedOr reports whether a disjunctive obligation (`Or(alts...)`)
// holds on varName at the current emission point. Delegates to the
// event-stream resolver; kept as a shim so in-place callers stay
// unchanged.
func (a *analyzer) dischargedOr(alts []Predicate, varName string) bool {
	return a.dischargedOrViaEvents(alts, varName)
}

// allRules returns the union of the current package's inference
// rules and those harvested from every imported package summary.
// Iteration order is deterministic per analyzer instance but
// package-import order (not topologically sorted) — rule search
// is exhaustive so order does not affect correctness.
func (a *analyzer) allRules() []InferRule {
	n := len(a.summary.Rules)
	for _, imp := range a.imports {
		n += len(imp.Rules)
	}
	if n == 0 {
		return nil
	}
	out := make([]InferRule, 0, n)
	out = append(out, a.summary.Rules...)
	for _, imp := range a.imports {
		out = append(out, imp.Rules...)
	}
	return out
}

// applyAssignPostconditions updates the fact set from an
// assignment statement whose RHS is a single call producing
// postcondition-bearing values:
//
//   - An annotated callee with ReturnPreds: every LHS identifier
//     gets each of those predicates as a fact. v1 does not try to
//     map return positions to LHS positions — a function annotated
//     with proven.Returns typically wraps a single return value,
//     and treating the predicates as applying to every LHS is a
//     safe over-approximation because the predicates are at least
//     true on the value that was actually wrapped.
//   - A prove.Must call: every LHS identifier gets a fact for each
//     of the predicates passed to prove.Must. prove.Must panics on
//     failure so the post-call path unconditionally has the fact.
//   - A prove.That call (returns (T, error)): handled by the
//     subsequent `if err != nil { escape }` guard, not here.
//     applyAssignPostconditions merely remembers the pending
//     prove.That for the guard walker below.
//
// The function is intentionally conservative: it touches facts
// only when the RHS is exactly one call expression, so `a, b :=
// f(), g()` and other compound forms leave the fact set untouched.
func (a *analyzer) applyAssignPostconditions(s *ast.AssignStmt) {
	if len(s.Rhs) != 1 {
		return
	}
	call, ok := s.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	// proven.Returns-annotated callee or analyzer-derived postconds.
	if calleePkg, key, ok := a.resolveCallee(call); ok {
		if sum, ok := a.lookupCalleeSummary(calleePkg, key); ok {
			leaves, ors := returnFacts(sum)
			if len(leaves) > 0 || len(ors) > 0 {
				for _, lhs := range s.Lhs {
					if k, ok := exprKey(lhs, a.imp); ok {
						for _, p := range leaves {
							a.rec.sourceLeaf(lhs.Pos(), p, k)
						}
						for _, alts := range ors {
							a.rec.sourceOr(lhs.Pos(), alts, k)
						}
					}
				}
			}
		}
	}
	// prove.Must(v, pred...) — unconditional postcondition on LHS.
	if a.isProveCall(call, "Must") {
		preds := a.proveCallPredicates(call, "Must")
		for _, lhs := range s.Lhs {
			if k, ok := exprKey(lhs, a.imp); ok {
				a.plantTrailingFacts(preds, k, lhs.Pos())
			}
		}
	}
	// trust.That(v, preds...) — local fact injection, no runtime
	// check. Every listed predicate becomes a fact on every LHS
	// identifier. The absence of a runtime side-effect is the
	// only semantic difference from prove.Must at the analyzer
	// level; the soundness of the assertion is the user's
	// responsibility.
	if a.isTrustCall(call, "That") {
		preds := a.trustCallPredicates(call)
		for _, lhs := range s.Lhs {
			if k, ok := exprKey(lhs, a.imp); ok {
				a.plantTrailingFacts(preds, k, lhs.Pos())
			}
		}
	}
	// prove.That(v, preds...) is intentionally NOT handled here.
	// Its postcondition only holds on the err == nil side of
	// the subsequent err-check guard, which spans two
	// statements. tryProvePattern in analyzeBlock owns that
	// pairing and plants the facts when it sees a matching
	// guard. An unpaired prove.That (no err-check following,
	// or the err is discarded with `_`) deliberately produces
	// no facts: the analyzer will not claim something the
	// program has not actually checked.
}

// applyDeclPostconditions mirrors applyAssignPostconditions for
// `var v = call(...)` declarations.
func (a *analyzer) applyDeclPostconditions(vs *ast.ValueSpec) {
	if len(vs.Values) != 1 {
		return
	}
	call, ok := vs.Values[0].(*ast.CallExpr)
	if !ok {
		return
	}
	if calleePkg, key, ok := a.resolveCallee(call); ok {
		if sum, ok := a.lookupCalleeSummary(calleePkg, key); ok {
			leaves, ors := returnFacts(sum)
			if len(leaves) > 0 || len(ors) > 0 {
				for _, name := range vs.Names {
					if name.Name != "_" {
						for _, p := range leaves {
							a.rec.sourceLeaf(name.Pos(), p, name.Name)
						}
						for _, alts := range ors {
							a.rec.sourceOr(name.Pos(), alts, name.Name)
						}
					}
				}
			}
		}
	}
	if a.isProveCall(call, "Must") {
		preds := a.proveCallPredicates(call, "Must")
		for _, name := range vs.Names {
			if name.Name != "_" {
				a.plantTrailingFacts(preds, name.Name, name.Pos())
			}
		}
	}
	if a.isTrustCall(call, "That") {
		preds := a.trustCallPredicates(call)
		for _, name := range vs.Names {
			if name.Name != "_" {
				a.plantTrailingFacts(preds, name.Name, name.Pos())
			}
		}
	}
}

// tryProvePattern recognizes a two-statement sequence of the form
//
//	v, err := prove.That(y, preds...)
//	if err <op> nil { ... } [else { ... }]
//
// and plants the postcondition facts where the semantics of the
// guard make them true:
//
//	op=!=  then-branch escapes → facts live after the if.
//	op=!=  neither branch escapes → facts live in the else branch only.
//	op===  then-branch gets facts; after the if, facts survive
//	       only if the else branch always escapes.
//
// Returns true iff both statements were consumed; the caller
// advances by two. Returns false without side-effects if the
// assignment is not a prove.That call, the next statement is not
// a matching err-check, or the err variable is the blank
// identifier (nothing to correlate against).
func (a *analyzer) tryProvePattern(assignStmt, guardStmt ast.Stmt) bool {
	pa := a.detectProveAssign(assignStmt)
	if pa == nil {
		return false
	}
	ifStmt, ok := guardStmt.(*ast.IfStmt)
	if !ok {
		return false
	}
	cond := classifyErrCond(ifStmt.Cond, pa.errVar)
	if cond == errCondUnknown {
		return false
	}

	// Consume the assignment (walk RHS for nested calls). No
	// facts are emitted here — applyAssignPostconditions no
	// longer handles prove.That.
	a.analyzeStmt(assignStmt)

	guardFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, false, guardFacts)
	negFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, true, negFacts)

	// Decide per branch whether prove facts apply.
	thenHasProve := cond == errCondIsNil
	elseHasProve := cond == errCondNotNil

	// then-branch
	thenScope := a.rec.enterScope()
	a.emitFactSetAsSources(guardFacts, ifStmt.Cond.Pos())
	if thenHasProve {
		a.emitProveAssignAsSources(pa, ifStmt.Cond.Pos())
	}
	a.analyzeBlock(ifStmt.Body)
	a.rec.leaveScope(thenScope)
	thenEscapes := blockAlwaysEscapes(ifStmt.Body)

	// else-branch (if any)
	elseEscapes := false
	if ifStmt.Else != nil {
		elseScope := a.rec.enterScope()
		a.emitFactSetAsSources(negFacts, ifStmt.Cond.Pos())
		if elseHasProve {
			a.emitProveAssignAsSources(pa, ifStmt.Cond.Pos())
		}
		switch e := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			a.analyzeBlock(e)
			elseEscapes = blockAlwaysEscapes(e)
		case *ast.IfStmt:
			a.analyzeIf(e)
		}
		a.rec.leaveScope(elseScope)
	}

	// Post-if join: when exactly one branch always escapes, the
	// surviving branch's guard-facts (plus the prove-facts if that
	// side had them) persist into the enclosing scope as if the
	// escaping branch had never been taken.
	postPos := ifStmt.End()
	switch {
	case thenEscapes && !elseEscapes:
		a.emitFactSetAsSources(negFacts, postPos)
		if elseHasProve {
			a.emitProveAssignAsSources(pa, postPos)
		}
	case !thenEscapes && elseEscapes:
		a.emitFactSetAsSources(guardFacts, postPos)
		if thenHasProve {
			a.emitProveAssignAsSources(pa, postPos)
		}
	}
	return true
}

// emitProveAssignAsSources plants pa's predicates as Source events
// at pos in the current scope, once per tracked target. A target is
// any canonical key where the successfully-proved value lives: the
// LHS value ident (when non-blank) and the first-arg canonical key
// (when the argument was keyable). Both point at the same runtime
// value, so the analyzer can discharge a downstream call that uses
// either name — most importantly, this enables
//
//	_, err := prove.That(holder.Value, proven.NonNil)
//	if err != nil { return }
//	needsNonNil(holder.Value)
//
// where the value LHS is blank and the fact must ride the first-arg
// key to reach the next use.
func (a *analyzer) emitProveAssignAsSources(pa *proveAssign, pos token.Pos) {
	for _, target := range pa.targets {
		for _, p := range pa.leaves {
			a.rec.sourceLeaf(pos, p, target)
		}
		for _, alts := range pa.ors {
			a.rec.sourceOr(pos, alts, target)
		}
	}
}

// proveAssign captures a recognized `v, err := prove.That(y,
// preds...)` assignment. targets holds every canonical key where
// the preds hold on the err == nil side: the LHS value ident if
// non-blank, and the first-arg key if that argument canonicalizes
// (a bare identifier or identifier-rooted selector chain). Either
// target is sufficient for downstream discharge; both can be present
// when `v := prove.That(x.F, p)` binds a new ident while the
// original selector path also continues to hold the predicate.
type proveAssign struct {
	errVar  string
	targets []string
	leaves  []Predicate
	ors     [][]Predicate
}

// detectProveAssign recognizes an assignment statement that binds
// two LHS positions to a prove.That call and returns the associated
// proveAssign. The error LHS must be a non-blank identifier — the
// err-check pairing in tryProvePattern has nothing to correlate
// against otherwise. The value LHS may be blank; in that case the
// pattern still tracks the first-arg canonical key as the plant
// target, so _, err := prove.That(expr, p) still rides p as a fact
// on expr once the err-check clears. If neither the value LHS nor
// the first-arg is trackable, the pattern is declined.
func (a *analyzer) detectProveAssign(stmt ast.Stmt) *proveAssign {
	as, ok := stmt.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
		return nil
	}
	errID, ok := as.Lhs[1].(*ast.Ident)
	if !ok || errID.Name == "_" {
		return nil
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return nil
	}
	if !a.isProveCall(call, "That") {
		return nil
	}
	var targets []string
	if valueID, ok := as.Lhs[0].(*ast.Ident); ok && valueID.Name != "_" {
		targets = append(targets, valueID.Name)
	}
	if len(call.Args) > 0 {
		if srcKey, ok := exprKey(call.Args[0], a.imp); ok {
			if !slices.Contains(targets, srcKey) {
				targets = append(targets, srcKey)
			}
		}
	}
	if len(targets) == 0 {
		return nil
	}
	preds := a.proveCallPredicates(call, "That")
	return &proveAssign{
		errVar:  errID.Name,
		targets: targets,
		leaves:  preds.Leaves,
		ors:     preds.Ors,
	}
}

type errCondKind int

const (
	errCondUnknown errCondKind = iota
	errCondNotNil              // `err != nil`
	errCondIsNil               // `err == nil`
)

// classifyErrCond inspects an if-condition for the canonical
// err == nil / err != nil comparison against the given err
// variable. Compound conditions (&& / || chains) are not
// recognized at v1 — the idiom under analysis is always a simple
// binary comparison.
func classifyErrCond(cond ast.Expr, errVar string) errCondKind {
	be, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return errCondUnknown
	}
	matchesErr := isIdent(be.X, errVar) || isIdent(be.Y, errVar)
	matchesNil := isNilIdent(be.X) || isNilIdent(be.Y)
	if !matchesErr || !matchesNil {
		return errCondUnknown
	}
	switch be.Op {
	case token.NEQ:
		return errCondNotNil
	case token.EQL:
		return errCondIsNil
	}
	return errCondUnknown
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// isProveCall reports whether call is `<proveAlias>.<which>(...)`.
func (a *analyzer) isProveCall(call *ast.CallExpr, which string) bool {
	if a.proveAlias == "" {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != a.proveAlias {
		return false
	}
	return sel.Sel.Name == which
}

// TrailingPreds carries the result of resolving the trailing
// predicate list of a prove.That / prove.Must / trust.That call.
// Leaves are leaf / And-decomposed predicates; Ors are disjunctive
// facts (from inline proven.Or(...)) that should be planted on the
// same LHS variable.
type TrailingPreds struct {
	Leaves []Predicate
	Ors    [][]Predicate
}

// proveCallPredicates returns the predicates passed after the
// value argument to a prove.That or prove.Must call. Unresolvable
// predicate arguments emit a strict-mode diagnostic via the
// analyzer's diags channel (when non-nil).
func (a *analyzer) proveCallPredicates(call *ast.CallExpr, which string) TrailingPreds {
	return a.resolveTrailingPredicates(call, "argument to prove."+which)
}

// isTrustCall reports whether call is `<trustAlias>.<which>(...)`.
// Mirrors isProveCall for the parallel trust package.
func (a *analyzer) isTrustCall(call *ast.CallExpr, which string) bool {
	if a.trustAlias == "" {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != a.trustAlias {
		return false
	}
	return sel.Sel.Name == which
}

// trustCallPredicates returns the predicates passed after the
// value argument to a trust.That call. Unresolvable predicate
// arguments emit a strict-mode diagnostic (when the analyzer was
// constructed with a diags channel).
func (a *analyzer) trustCallPredicates(call *ast.CallExpr) TrailingPreds {
	return a.resolveTrailingPredicates(call, "argument to trust.That")
}

// resolveTrailingPredicates is the common body behind
// proveCallPredicates and trustCallPredicates: both `pkg.That(v,
// preds...)` shapes take the value as the first argument and
// predicates as every subsequent argument. Unresolvable predicate
// arguments are reported via the analyzer's diagnostic channel
// (when non-nil) using the caller-supplied role label so the
// message identifies which site is the problem.
//
// Inline proven.And decomposes here too: prove.Must(raw,
// proven.And(a, b)) plants both leaf facts a and b on the LHS of
// the assignment, same as prove.Must(raw, a, b). Nested And
// flattens fully.
func (a *analyzer) resolveTrailingPredicates(call *ast.CallExpr, role string) TrailingPreds {
	var out TrailingPreds
	if len(call.Args) < 2 {
		return out
	}
	for _, arg := range call.Args[1:] {
		leaves, ors, ok := resolveAndFlat(arg, a.imp, a.summary.ImportPath, a.fset, a.diags, role)
		if !ok {
			continue
		}
		out.Leaves = append(out.Leaves, leaves...)
		out.Ors = append(out.Ors, ors...)
	}
	return out
}

// aliasFor returns the import alias under which importPath is
// imported in this file, or "" if not imported.
func (imp *importInfo) aliasFor(importPath string) string {
	for alias, p := range imp.aliases {
		if p == importPath {
			return alias
		}
	}
	return ""
}

// analyzeSource parses a single source file, runs the Phase 2
// scanner against it to build a summary, then walks every
// FuncDecl and returns all call-site discharges. Convenience for
// tests; production code uses AnalyzePackage which handles
// multi-file packages in one pass.
func analyzeSource(importPath, src string) ([]CallDischarge, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "input.go", src, 0)
	if err != nil {
		return nil, err
	}
	imp := collectImports(f)
	sum := &PackageSummary{
		ImportPath: importPath,
		Funcs:      make(map[string]*FuncSummary),
	}
	scanFile(sum, importPath, fset, f, nil)
	var all []CallDischarge
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			all = append(all, AnalyzeFunc(fn, sum, imp, nil, fset, nil)...)
		}
	}
	return all, nil
}

// AnalyzePackage parses each source file once, builds the package
// summary (Phase 2), then walks every FuncDecl in every file and
// collects the call-site discharges (Phase 3/4). Returns the
// summary, the flat list of discharges across the package, and
// the FileSet that owns the Pos values inside each discharge —
// callers need the same fset to resolve Pos to (file, line, col)
// for diagnostics.
//
// Short-circuits packages that need no analysis: if the scan
// finds no obligations (summary.Funcs empty) the analyzer is
// skipped entirely — no caller in the package can reference an
// obligation that is not in summary.Funcs. This is the common
// case for the many stdlib and non-proven-using user packages
// that pass through the preprocessor on every build.
func AnalyzePackage(importPath string, sources []string) (*PackageSummary, []CallDischarge, *token.FileSet, error) {
	fset := token.NewFileSet()
	sum := &PackageSummary{
		ImportPath: importPath,
		Funcs:      make(map[string]*FuncSummary),
	}
	files := make([]*ast.File, 0, len(sources))
	for _, src := range sources {
		f, err := parser.ParseFile(fset, src, nil, 0)
		if err != nil {
			return nil, nil, nil, err
		}
		files = append(files, f)
	}
	for _, f := range files {
		scanFile(sum, importPath, fset, f, nil)
	}
	if len(sum.Funcs) == 0 {
		return sum, nil, fset, nil
	}
	var all []CallDischarge
	for _, f := range files {
		imp := collectImports(f)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				all = append(all, AnalyzeFunc(fn, sum, imp, nil, fset, nil)...)
			}
		}
	}
	return sum, all, fset, nil
}

// dischargeForCallee returns the discharge entry for the call to
// calleeKey in calls, or nil if not present. Helper for tests.
func dischargeForCallee(calls []CallDischarge, calleeKey string) *CallDischarge {
	for i := range calls {
		if calls[i].CalleeKey == calleeKey {
			return &calls[i]
		}
	}
	return nil
}
