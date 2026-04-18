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
	"go/ast"
	"go/parser"
	"go/token"
)

// proveImportPath is the runtime-boundary-validation package
// whose successful calls the analyzer treats as fact sources.
const proveImportPath = "github.com/GiGurra/proven/pkg/prove"

// Fact asserts that predicate Pred holds on the variable named Var
// at a given program point.
type Fact struct {
	Pred Predicate
	Var  string
}

// FactSet is an unordered set of facts. The underlying map is not
// exposed; callers use Add, Has, and Clone.
type FactSet struct {
	m map[Fact]struct{}
}

func newFactSet() *FactSet {
	return &FactSet{m: make(map[Fact]struct{})}
}

func (fs *FactSet) Add(f Fact)                         { fs.m[f] = struct{}{} }
func (fs *FactSet) Has(pred Predicate, v string) bool  { _, ok := fs.m[Fact{Pred: pred, Var: v}]; return ok && v != "" }
func (fs *FactSet) Clone() *FactSet {
	out := newFactSet()
	for f := range fs.m {
		out.m[f] = struct{}{}
	}
	return out
}

// CallDischarge records the obligation-discharge status for one
// call site analyzed against the caller's flow facts.
type CallDischarge struct {
	Pos       token.Pos
	CalleeKey string
	Params    []ParamDischarge
}

// Undischarged reports whether any parameter of this call site
// still has a missing predicate after flow analysis.
func (c CallDischarge) Undischarged() bool {
	for _, p := range c.Params {
		if len(p.Missing) > 0 {
			return true
		}
	}
	return false
}

// ParamDischarge records per-parameter discharge: which predicates
// the callee required (from Phase 2's summary) and which remained
// missing from the caller's fact set at the call site.
type ParamDischarge struct {
	ParamIdx int
	ArgName  string
	Required []Predicate
	Missing  []Predicate
}

// AnalyzeFunc walks caller's body and returns one CallDischarge
// per call to a function whose key appears in summary.Funcs.
// Callees without a summary entry (external, or annotated only
// via proven.Returns with no ParamPreds) produce no entries.
//
// The FuncDecl must belong to the package whose summary is passed;
// cross-package analysis is Phase 6.
func AnalyzeFunc(caller *ast.FuncDecl, summary *PackageSummary, imp *importInfo) []CallDischarge {
	if caller.Body == nil {
		return nil
	}
	a := &analyzer{
		summary:     summary,
		imp:         imp,
		facts:       newFactSet(),
		proveAlias:  imp.aliasFor(proveImportPath),
		provenAlias: imp.provenAlias,
	}
	a.analyzeBlock(caller.Body)
	return a.out
}

// analyzer owns the mutable fact set, the read-only summary/import
// context, and the growing list of discharge results.
type analyzer struct {
	summary     *PackageSummary
	imp         *importInfo
	facts       *FactSet
	out         []CallDischarge
	proveAlias  string
	provenAlias string
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
func (a *analyzer) analyzeBlock(block *ast.BlockStmt) {
	for _, stmt := range block.List {
		a.analyzeStmt(stmt)
	}
}

func (a *analyzer) analyzeStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		a.analyzeIf(s)
	case *ast.ExprStmt:
		a.walkCalls(s.X)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			a.walkCalls(rhs)
		}
		a.applyAssignPostconditions(s)
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
	case *ast.BlockStmt:
		saved := a.facts.Clone()
		a.analyzeBlock(s)
		a.facts = saved
	case *ast.ForStmt:
		saved := a.facts.Clone()
		if s.Body != nil {
			a.analyzeBlock(s.Body)
		}
		a.facts = saved
	case *ast.RangeStmt:
		saved := a.facts.Clone()
		if s.Body != nil {
			a.analyzeBlock(s.Body)
		}
		a.facts = saved
	case *ast.SwitchStmt:
		saved := a.facts.Clone()
		if s.Body != nil {
			for _, cc := range s.Body.List {
				if cl, ok := cc.(*ast.CaseClause); ok {
					for _, cs := range cl.Body {
						a.analyzeStmt(cs)
					}
				}
			}
		}
		a.facts = saved
	}
}

// analyzeIf handles the two if-statement patterns that establish
// facts:
//
//   1. `if guard { body }` — facts implied by guard hold inside
//      body. On exit, if body always escapes, the negation of
//      guard's facts persists to subsequent statements.
//   2. `if guard { body } else { elseBody }` — guard-facts hold
//      inside body, their negation inside elseBody. If exactly
//      one branch escapes, the other's surviving facts persist.
//
// The escape check recognizes ReturnStmt and calls to panic as
// unconditional exits; anything else is treated as fall-through
// for conservatism (false negatives are safe here — they just
// reduce what the analyzer can discharge, never what it accepts).
func (a *analyzer) analyzeIf(ifStmt *ast.IfStmt) {
	saved := a.facts.Clone()
	guardFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, false, guardFacts)

	// then-branch sees current facts ∪ guardFacts.
	a.facts = unionFacts(saved, guardFacts)
	a.analyzeBlock(ifStmt.Body)
	thenEscapes := blockAlwaysEscapes(ifStmt.Body)

	// Optional else-branch: collect negated facts (only useful for
	// "if !pred(x)" shape — collectGuardFacts knows how to negate).
	elseEscapes := false
	negFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, true, negFacts)
	if ifStmt.Else != nil {
		a.facts = unionFacts(saved, negFacts)
		switch e := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			a.analyzeBlock(e)
			elseEscapes = blockAlwaysEscapes(e)
		case *ast.IfStmt:
			a.analyzeIf(e)
			// Nested if-else chains: conservatively treat as
			// non-escaping so later facts are not over-claimed.
		}
	}

	// Post-if fact set:
	switch {
	case thenEscapes && !elseEscapes:
		// then always exits; the continuation is the else-path,
		// whose facts are the negated guard.
		a.facts = unionFacts(saved, negFacts)
	case !thenEscapes && elseEscapes:
		// else always exits; the continuation is the then-path.
		a.facts = unionFacts(saved, guardFacts)
	default:
		// Neither or both escape: nothing can be concluded about
		// the joined state, so restore to pre-if facts.
		a.facts = saved
	}
}

// unionFacts returns a new FactSet containing every fact in a or b.
func unionFacts(a, b *FactSet) *FactSet {
	out := a.Clone()
	for f := range b.m {
		out.m[f] = struct{}{}
	}
	return out
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

// callAsPredicate matches a call expression of the form
// `predIdent(x)` or `pkgAlias.Pred(x)` where x is a single
// identifier, and returns the resolved predicate identity plus
// the subject variable name.
func callAsPredicate(call *ast.CallExpr, imp *importInfo, importPath string) (Predicate, string, bool) {
	if len(call.Args) != 1 {
		return Predicate{}, "", false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return Predicate{}, "", false
	}
	pred, ok := resolvePredicate(call.Fun, imp, importPath)
	if !ok {
		return Predicate{}, "", false
	}
	return pred, arg.Name, true
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
// so obligations on inner calls are checked.
func (a *analyzer) walkCalls(expr ast.Expr) {
	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			a.recordCallDischarge(call)
		}
		return true
	})
}

// recordCallDischarge looks up the callee's summary (if any) and
// produces a CallDischarge for the call site. Callees whose name
// is not in summary.Funcs produce no entry: either they are not
// annotated (nothing to discharge) or they are external (Phase 6
// scope).
func (a *analyzer) recordCallDischarge(call *ast.CallExpr) {
	key := calleeKey(call)
	if key == "" {
		return
	}
	sum, ok := a.summary.Funcs[key]
	if !ok {
		return
	}
	var params []ParamDischarge
	for idx, required := range sum.ParamPreds {
		if idx >= len(call.Args) {
			continue
		}
		argName, _ := identName(call.Args[idx])
		var missing []Predicate
		for _, p := range required {
			if !a.discharged(p, argName) {
				missing = append(missing, p)
			}
		}
		params = append(params, ParamDischarge{
			ParamIdx: idx,
			ArgName:  argName,
			Required: append([]Predicate(nil), required...),
			Missing:  missing,
		})
	}
	if len(params) == 0 {
		return
	}
	a.out = append(a.out, CallDischarge{
		Pos:       call.Pos(),
		CalleeKey: key,
		Params:    params,
	})
}

// discharged reports whether predicate pred holds on the
// identifier named varName, either as a direct fact or by
// implication through the declared inference rules.
//
// An implication chain succeeds when a rule whose To is pred can
// be closed: its From predicate holds on varName (possibly
// through another chain) and, when present, its Given context
// also holds on varName. Unknown / not-in-scope variable names
// ("" from non-ident arguments) never discharge anything.
//
// Cycle-safe: a visited set collects every predicate the query
// has already attempted to prove on this variable, so a
// pathological A ⇒ B ⇒ A ruleset returns false without
// recursing forever. The visited marker is per top-level query,
// so an independent later query starts fresh.
func (a *analyzer) discharged(pred Predicate, varName string) bool {
	if varName == "" {
		return false
	}
	return a.dischargedRec(pred, varName, make(map[Predicate]bool))
}

func (a *analyzer) dischargedRec(pred Predicate, varName string, visited map[Predicate]bool) bool {
	if a.facts.Has(pred, varName) {
		return true
	}
	if visited[pred] {
		return false
	}
	visited[pred] = true

	for _, rule := range a.summary.Rules {
		if rule.To != pred {
			continue
		}
		if !a.dischargedRec(rule.From, varName, visited) {
			continue
		}
		if rule.Given != nil && !a.dischargedRec(*rule.Given, varName, visited) {
			continue
		}
		return true
	}
	return false
}

// calleeKey returns the summary key for a call whose callee we can
// resolve: bare identifiers (same-package free functions or
// package-level variables) yield the identifier name. Selector
// expressions (method calls, cross-package calls) return "" —
// those are outside Phase 3's scope.
func calleeKey(call *ast.CallExpr) string {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
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
	// proven.Returns-annotated callee.
	if key := calleeKey(call); key != "" {
		if sum, ok := a.summary.Funcs[key]; ok && len(sum.ReturnPreds) > 0 {
			for _, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					for _, p := range sum.ReturnPreds {
						a.facts.Add(Fact{Pred: p, Var: id.Name})
					}
				}
			}
		}
	}
	// prove.Must(v, pred...) — unconditional postcondition on LHS.
	if a.isProveCall(call, "Must") {
		preds := a.proveCallPredicates(call)
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: id.Name})
				}
			}
		}
	}
	// prove.That(v, pred...) — conditional on err == nil. The
	// guard that establishes the fact is the following if-stmt,
	// handled by analyzeIf via its escape detection plus the
	// pending-prove-that tracking below. For v1 we emit the
	// fact on the value LHS immediately and rely on the test
	// fixtures to include the err-check guard — a correct
	// caller always pairs the two. The preprocessor errs toward
	// over-claiming here; Phase 5's rewriter will only act when
	// the enclosing analysis produces no Missing entries, so a
	// forgotten err-check manifests as a runtime failure rather
	// than a silent misdischarge.
	//
	// A stricter implementation would require the analyzer to
	// see the err-check before emitting the fact; recorded as a
	// known limitation in roadmap risks and left for a follow-up.
	if a.isProveCall(call, "That") {
		preds := a.proveCallPredicates(call)
		if len(s.Lhs) >= 1 {
			if id, ok := s.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: id.Name})
				}
			}
		}
	}
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
	if key := calleeKey(call); key != "" {
		if sum, ok := a.summary.Funcs[key]; ok && len(sum.ReturnPreds) > 0 {
			for _, name := range vs.Names {
				if name.Name != "_" {
					for _, p := range sum.ReturnPreds {
						a.facts.Add(Fact{Pred: p, Var: name.Name})
					}
				}
			}
		}
	}
	if a.isProveCall(call, "Must") {
		preds := a.proveCallPredicates(call)
		for _, name := range vs.Names {
			if name.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: name.Name})
				}
			}
		}
	}
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

// proveCallPredicates returns the predicates passed after the
// value argument to a prove.That or prove.Must call. Args that
// cannot be resolved to a Predicate are skipped, matching the
// scanner's policy.
func (a *analyzer) proveCallPredicates(call *ast.CallExpr) []Predicate {
	if len(call.Args) < 2 {
		return nil
	}
	var out []Predicate
	for _, arg := range call.Args[1:] {
		if p, ok := resolvePredicate(arg, a.imp, a.summary.ImportPath); ok {
			out = append(out, p)
		}
	}
	return out
}

// identName returns the name of an identifier expression and
// whether the expression was a direct identifier.
func identName(e ast.Expr) (string, bool) {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name, true
	}
	return "", false
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
// tests; production callers drive scanner and analyzer
// separately because they need to aggregate summaries across
// multiple files.
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
	scanFile(sum, importPath, f)
	var all []CallDischarge
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			all = append(all, AnalyzeFunc(fn, sum, imp)...)
		}
	}
	return all, nil
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
