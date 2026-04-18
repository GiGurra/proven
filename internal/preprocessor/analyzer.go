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

// FactSet is an unordered set of facts. The underlying map is not
// exposed; callers use Add, Has, and Clone.
type FactSet struct {
	m map[Fact]struct{}
}

func newFactSet() *FactSet {
	return &FactSet{m: make(map[Fact]struct{})}
}

func (fs *FactSet) Add(f Fact) { fs.m[f] = struct{}{} }
func (fs *FactSet) Has(pred Predicate, v string) bool {
	_, ok := fs.m[Fact{Pred: pred, Var: v}]
	return ok && v != ""
}
func (fs *FactSet) Clone() *FactSet {
	out := newFactSet()
	for f := range fs.m {
		out.m[f] = struct{}{}
	}
	return out
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
	if caller.Body == nil {
		return nil
	}
	a := &analyzer{
		summary:     summary,
		imp:         imp,
		imports:     imports,
		facts:       seedFactsFromPreconditions(caller, summary),
		proveAlias:  imp.aliasFor(proveImportPath),
		trustAlias:  imp.aliasFor(trustImportPath),
		provenAlias: imp.provenAlias,
		fset:        fset,
		diags:       diags,
	}
	a.analyzeBlock(caller.Body)
	return a.out
}

// seedFactsFromPreconditions returns the initial FactSet for
// analyzing caller's body. Each `proven.That(param, pred)` at the
// top of caller's body — which the scanner already recorded in
// summary.Funcs[caller.key].ParamPreds[i] — is a precondition every
// caller has already discharged at its call site, so inside
// caller's body the predicate holds on the corresponding parameter
// as a starting fact. Seeding this lets two things work:
//
//  1. `return proven.Returns(x, pred)` validates against the
//     function's own declared precondition on x.
//  2. A function that declares a precondition and internally
//     forwards the parameter to another function requiring the
//     same precondition discharges without re-guarding.
//
// Functions without a summary entry (no obligations declared)
// start with an empty fact set — no facts to seed. Parameters
// whose declared preconditions refer to predicates the analyzer
// cannot find are still faithfully seeded; the point is that the
// caller has promised those predicates hold, irrespective of
// whether the local analyzer would accept the predicate identity
// elsewhere.
func seedFactsFromPreconditions(caller *ast.FuncDecl, summary *PackageSummary) *FactSet {
	facts := newFactSet()
	if summary == nil {
		return facts
	}
	key := funcDeclKey(caller)
	fsum, ok := summary.Funcs[key]
	if !ok || fsum == nil {
		return facts
	}
	paramNames := paramNamesByIndex(caller.Type)
	for idx, preds := range fsum.ParamPreds {
		name, ok := paramNames[idx]
		if !ok || name == "" || name == "_" {
			continue
		}
		for _, p := range preds {
			facts.Add(Fact{Pred: p, Var: name})
		}
	}
	return facts
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

// analyzer owns the mutable fact set, the read-only summary/import
// context (including any imported package summaries), and the
// growing list of discharge results.
type analyzer struct {
	summary     *PackageSummary
	imp         *importInfo
	imports     map[string]*PackageSummary
	facts       *FactSet
	out         []CallDischarge
	proveAlias  string
	trustAlias  string
	provenAlias string
	fset        *token.FileSet
	diags       *[]Diagnostic
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
		}
		return true
	})
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
		pred, ok := resolvePredicate(arg, a.imp, a.summary.ImportPath)
		if !ok {
			// The unresolvable-predicate diagnostic is already
			// emitted by the scanner's recordReturns pass; skip
			// here to avoid doubling the error.
			continue
		}
		if !a.discharged(pred, valueID.Name) {
			reportUnprovenReturns(a.diags, a.fset, call, pred, valueID.Name, a.summary.ImportPath)
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
		CalleePkg: calleePkg,
		CalleeKey: key,
		Params:    params,
	})
}

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

	for _, rule := range a.allRules() {
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
	// proven.Returns-annotated callee.
	if calleePkg, key, ok := a.resolveCallee(call); ok {
		if sum, ok := a.lookupCalleeSummary(calleePkg, key); ok && len(sum.ReturnPreds) > 0 {
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
		preds := a.proveCallPredicates(call, "Must")
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: id.Name})
				}
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
			if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: id.Name})
				}
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
		if sum, ok := a.lookupCalleeSummary(calleePkg, key); ok && len(sum.ReturnPreds) > 0 {
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
		preds := a.proveCallPredicates(call, "Must")
		for _, name := range vs.Names {
			if name.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: name.Name})
				}
			}
		}
	}
	if a.isTrustCall(call, "That") {
		preds := a.trustCallPredicates(call)
		for _, name := range vs.Names {
			if name.Name != "_" {
				for _, p := range preds {
					a.facts.Add(Fact{Pred: p, Var: name.Name})
				}
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

	saved := a.facts.Clone()
	guardFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, false, guardFacts)
	negFacts := newFactSet()
	collectGuardFacts(ifStmt.Cond, a.imp, a.summary.ImportPath, true, negFacts)

	// Decide per branch whether prove facts apply.
	thenHasProve := cond == errCondIsNil
	elseHasProve := cond == errCondNotNil

	// then-branch
	a.facts = unionFacts(saved, guardFacts)
	if thenHasProve {
		for _, f := range pa.facts {
			a.facts.Add(f)
		}
	}
	a.analyzeBlock(ifStmt.Body)
	thenEscapes := blockAlwaysEscapes(ifStmt.Body)

	// else-branch (if any)
	elseEscapes := false
	if ifStmt.Else != nil {
		a.facts = unionFacts(saved, negFacts)
		if elseHasProve {
			for _, f := range pa.facts {
				a.facts.Add(f)
			}
		}
		switch e := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			a.analyzeBlock(e)
			elseEscapes = blockAlwaysEscapes(e)
		case *ast.IfStmt:
			a.analyzeIf(e)
		}
	}

	// After-if fact set:
	//   - if the then-branch always escapes, the continuation
	//     corresponds to the else-side, so elseHasProve decides.
	//   - if the else-branch always escapes, the continuation
	//     is the then-side, so thenHasProve decides.
	//   - otherwise the control-flow merge is conservative and
	//     prove facts are dropped.
	switch {
	case thenEscapes && !elseEscapes:
		a.facts = unionFacts(saved, negFacts)
		if elseHasProve {
			for _, f := range pa.facts {
				a.facts.Add(f)
			}
		}
	case !thenEscapes && elseEscapes:
		a.facts = unionFacts(saved, guardFacts)
		if thenHasProve {
			for _, f := range pa.facts {
				a.facts.Add(f)
			}
		}
	default:
		a.facts = saved
	}
	return true
}

// proveAssign captures the essentials of a recognized
// `v, err := prove.That(y, preds...)` assignment: the identifiers
// bound to the value and the error, and the facts the pattern
// implies on the err == nil side.
type proveAssign struct {
	errVar string
	facts  []Fact
}

// detectProveAssign recognizes an assignment statement that binds
// two identifiers (value, error) to a prove.That call and returns
// the associated proveAssign. Blank identifiers disqualify the
// pattern — there is nothing to pair the err-check against if the
// caller discarded the error.
func (a *analyzer) detectProveAssign(stmt ast.Stmt) *proveAssign {
	as, ok := stmt.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
		return nil
	}
	valueID, ok := as.Lhs[0].(*ast.Ident)
	if !ok || valueID.Name == "_" {
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
	preds := a.proveCallPredicates(call, "That")
	facts := make([]Fact, 0, len(preds))
	for _, p := range preds {
		facts = append(facts, Fact{Pred: p, Var: valueID.Name})
	}
	return &proveAssign{errVar: errID.Name, facts: facts}
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

// proveCallPredicates returns the predicates passed after the
// value argument to a prove.That or prove.Must call. Unresolvable
// predicate arguments emit a strict-mode diagnostic via the
// analyzer's diags channel (when non-nil).
func (a *analyzer) proveCallPredicates(call *ast.CallExpr, which string) []Predicate {
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
func (a *analyzer) trustCallPredicates(call *ast.CallExpr) []Predicate {
	return a.resolveTrailingPredicates(call, "argument to trust.That")
}

// resolveTrailingPredicates is the common body behind
// proveCallPredicates and trustCallPredicates: both `pkg.That(v,
// preds...)` shapes take the value as the first argument and
// predicates as every subsequent argument. Unresolvable predicate
// arguments are reported via the analyzer's diagnostic channel
// (when non-nil) using the caller-supplied role label so the
// message identifies which site is the problem.
func (a *analyzer) resolveTrailingPredicates(call *ast.CallExpr, role string) []Predicate {
	if len(call.Args) < 2 {
		return nil
	}
	var out []Predicate
	for _, arg := range call.Args[1:] {
		p, ok := resolvePredicate(arg, a.imp, a.summary.ImportPath)
		if !ok {
			reportBadPredicate(a.diags, a.fset, arg, role)
			continue
		}
		out = append(out, p)
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
