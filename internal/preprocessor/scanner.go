package preprocessor

// Phase 2: per-package obligation scan.
//
// For each .go source file the compiler was handed, discover which
// functions declare preconditions (proven.That) or postconditions
// (proven.Returns) and with which predicates. The output is an
// in-memory PackageSummary consumed by Phase 3's flow-sensitive
// discharge at call sites. Phase 6 will persist these summaries
// across package boundaries; for now everything lives within one
// preprocessor invocation.
//
// Scope intentionally narrow: v1 supports the direct-parameter-
// reference shape documented in docs/design.md ("Argument
// expressions"). When the scanner is called with a non-nil
// diagnostics channel (the production toolexec path), an
// unresolvable predicate expression (inline combinator,
// arbitrary expression, function literal) or a non-parameter
// subject at a proven.That site fails the build with a Go-standard
// `file:line:col:` diagnostic — the same "no silent bypass"
// principle the link gate enforces elsewhere. Passing diags == nil
// restores the old lenient behavior for stand-alone API callers
// (ScanPackage, tests) that do not surface diagnostics to the
// toolchain.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// provenImportPath is the import path of the pkg/proven package.
// The scanner treats calls on its alias as meaningful; everything
// else is ordinary user code.
const provenImportPath = "github.com/GiGurra/proven/pkg/proven"

// inferImportPath is the import path of the pkg/infer package.
// Package-scope `var _ = infer.From(p).[Given(c).]To(q)` chains
// in files that import it are harvested as InferRule entries on
// the PackageSummary.
const inferImportPath = "github.com/GiGurra/proven/pkg/infer"

// Predicate identifies a predicate function by its declaring
// package and identifier name. For predicates defined in the
// scanned package itself, Pkg is that package's own import path —
// giving every predicate a uniform qualified identity that Phase 3
// can compare without special-casing same-package lookup.
type Predicate struct {
	Pkg  string
	Name string
}

// FuncSummary collects the proven.That / proven.Returns facts
// declared in one function body.
//
// ParamPreds: parameter position (0-based) to the AND-composed
// list of leaf-predicate obligations asserted on that parameter by
// every proven.That call in the body. proven.And(a, b) and its
// nested forms flatten into additional leaves here. An entry is
// present only if at least one such call exists; absent parameters
// carry no obligations.
//
// ParamOrs: parameter position to the AND-composed list of
// disjunctive obligations, each inner []Predicate representing one
// proven.Or(a, b) call's OR-alternatives (the Or's arguments must
// be named leaves — nested combinators inside Or are rejected by
// strict mode). Any leaf holding, or an exact-match Or-fact, is
// enough to discharge one disjunction.
//
// ReturnPreds / ReturnOrs: the postcondition counterparts, merged
// across every proven.Returns / trust.Returns call in the body.
// Phase 2 treats these as bags because tracking "which return
// value position is constrained" would require flow analysis over
// the body, which is Phase 3's job.
//
// Recv is the receiver type identifier for methods (without the
// leading star), empty for free functions.
type FuncSummary struct {
	Name        string
	Recv        string
	ParamPreds  map[int][]Predicate
	ParamOrs    map[int][][]Predicate
	ReturnPreds []Predicate
	ReturnOrs   [][]Predicate

	// DerivedReturnPreds / DerivedReturnOrs are postconditions the
	// analyzer inferred from the body — the intersection of fact sets
	// on the returned identifier across every ReturnStmt. Populated
	// after the flow-sensitive pass (AnalyzeFunc). Callers plant them
	// as facts at assignment sites the same way as the explicit
	// ReturnPreds / ReturnOrs above, so a function without an
	// explicit proven.Returns can still advertise the facts its body
	// actually proves. Keeping the explicit fields distinct preserves
	// the "this is the declared contract" marker for API-boundary
	// functions that opt into a claim the compiler verifies.
	DerivedReturnPreds []Predicate
	DerivedReturnOrs   [][]Predicate
}

// Key returns a stable identifier for looking up the summary from
// a call site: "Func" for free functions, "Recv.Method" for methods.
func (s *FuncSummary) Key() string {
	if s.Recv == "" {
		return s.Name
	}
	return s.Recv + "." + s.Name
}

// InferRule is one declared package-scope implication rule
// harvested from `var _ = infer.From(premises...).[Given(contexts...).]To(conclusions...)`.
//
// Every slot is variadic and AND-composed — the rule reads as
// "if every From predicate AND every Given predicate holds on the
// variable, then every To predicate holds on it". Given is empty
// when no .Given(...) step was present.
//
// Rules are trusted — the scanner does not verify that the
// implication actually holds (docs/design.md). Unresolvable
// predicate arguments (combinator calls, function literals, etc.)
// cause the rule to be skipped rather than silently stored with
// an empty identity.
type InferRule struct {
	From  []Predicate
	Given []Predicate
	To    []Predicate
}

// PackageSummary is the scanner output for one compile unit. Only
// functions with at least one obligation are present in Funcs.
type PackageSummary struct {
	ImportPath string
	Funcs      map[string]*FuncSummary
	Rules      []InferRule
}

// ScanPackage parses each source file and returns the obligation
// summary for the package. importPath is the Go import path of the
// package being scanned (from `compile -p <importpath>`); it
// populates Predicate.Pkg for same-package predicate references.
//
// ScanPackage is tolerant of source files that do not import
// pkg/proven: they produce no entries. A syntax error in any file
// aborts the scan with a wrapped error, matching the compile
// step's fail-fast behavior.
func ScanPackage(importPath string, sources []string) (*PackageSummary, error) {
	sum := &PackageSummary{
		ImportPath: importPath,
		Funcs:      make(map[string]*FuncSummary),
	}
	fset := token.NewFileSet()
	for _, src := range sources {
		f, err := parser.ParseFile(fset, src, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", src, err)
		}
		scanFile(sum, importPath, fset, f, nil)
	}
	return sum, nil
}

// scanFile walks one parsed file's top-level declarations.
// FuncDecls with proven.That / proven.Returns calls populate
// sum.Funcs; package-scope `var _ = infer.From(...)` chains
// populate sum.Rules. Files that import neither pkg/proven nor
// pkg/infer are skipped early.
//
// diags (when non-nil) collects strict-mode diagnostics the scanner
// produces when a proven.That / proven.Returns / infer.From.To site
// cannot resolve its predicate argument to a named func or pkg.Name
// selector. Passing nil disables strict-mode reporting for
// stand-alone API calls (ScanPackage) that do not surface
// diagnostics to the toolchain.
func scanFile(sum *PackageSummary, importPath string, fset *token.FileSet, f *ast.File, diags *[]Diagnostic) {
	imp := collectImports(f)
	inferAlias := imp.aliasFor(inferImportPath)
	trustAlias := imp.aliasFor(trustImportPath)
	if imp.provenAlias == "" && inferAlias == "" && trustAlias == "" {
		return
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil || (imp.provenAlias == "" && trustAlias == "") {
				continue
			}
			if summary := scanFunc(d, imp, importPath, fset, diags); summary != nil {
				sum.Funcs[summary.Key()] = summary
			}
		case *ast.GenDecl:
			if d.Tok != token.VAR || inferAlias == "" {
				continue
			}
			scanInferRules(sum, imp, importPath, inferAlias, d, fset, diags)
		}
	}
}

// scanInferRules examines each ValueSpec value in genDecl for an
// inference-rule chain and appends any matches to sum.Rules.
func scanInferRules(sum *PackageSummary, imp *importInfo, importPath, inferAlias string, genDecl *ast.GenDecl, fset *token.FileSet, diags *[]Diagnostic) {
	for _, spec := range genDecl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, val := range vs.Values {
			if rule, ok := extractInferRule(val, inferAlias, imp, importPath, fset, diags); ok {
				sum.Rules = append(sum.Rules, rule)
			}
		}
	}
}

// extractInferRule matches one of the two fluent shapes:
//
//	infer.From(premises...).To(conclusions...)
//	infer.From(premises...).Given(contexts...).To(conclusions...)
//
// Every slot is variadic and AND-composed: multiple premises mean
// every premise must hold, multiple conclusions mean every conclusion
// follows, multiple contexts AND the Given filter. Structural
// mismatches (a call that does not match either fluent shape at all)
// silently return (_, false) — they are not rules. An empty slot
// (e.g. `infer.From().To(q)`) is treated as a structural mismatch.
//
// Predicate arguments that ARE in a matching rule shape but cannot
// be resolved to a named func or pkg.Name selector emit a strict-
// mode diagnostic via diags, so function literals and inline
// combinator calls at rule-declaration sites fail the build instead
// of being silently dropped. Pass diags == nil to keep the old
// lenient behavior (used by ScanPackage, a standalone API not in
// the toolchain path).
func extractInferRule(expr ast.Expr, inferAlias string, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic) (InferRule, bool) {
	toCall, ok := expr.(*ast.CallExpr)
	if !ok {
		return InferRule{}, false
	}
	toSel, ok := toCall.Fun.(*ast.SelectorExpr)
	if !ok || toSel.Sel.Name != "To" || len(toCall.Args) == 0 {
		return InferRule{}, false
	}

	inner, ok := toSel.X.(*ast.CallExpr)
	if !ok {
		return InferRule{}, false
	}
	innerSel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok {
		return InferRule{}, false
	}

	// We have committed to this being an infer rule: any unresolvable
	// predicate argument below emits a strict-mode diagnostic.
	conclusions, ok := resolvePredicates(toCall.Args, imp, importPath, diags, fset, "conclusion of infer rule")
	if !ok {
		return InferRule{}, false
	}

	switch innerSel.Sel.Name {
	case "From":
		// infer.From(premises...).To(conclusions...)
		if !isInferIdent(innerSel.X, inferAlias) {
			return InferRule{}, false
		}
		if len(inner.Args) == 0 {
			return InferRule{}, false
		}
		premises, ok := resolvePredicates(inner.Args, imp, importPath, diags, fset, "premise of infer.From")
		if !ok {
			return InferRule{}, false
		}
		return InferRule{From: premises, To: conclusions}, true

	case "Given":
		// infer.From(premises...).Given(contexts...).To(conclusions...)
		if len(inner.Args) == 0 {
			return InferRule{}, false
		}
		fromCall, ok := innerSel.X.(*ast.CallExpr)
		if !ok {
			return InferRule{}, false
		}
		fromSel, ok := fromCall.Fun.(*ast.SelectorExpr)
		if !ok || fromSel.Sel.Name != "From" || len(fromCall.Args) == 0 {
			return InferRule{}, false
		}
		if !isInferIdent(fromSel.X, inferAlias) {
			return InferRule{}, false
		}
		contexts, ok := resolvePredicates(inner.Args, imp, importPath, diags, fset, "context of infer.Given")
		if !ok {
			return InferRule{}, false
		}
		premises, ok := resolvePredicates(fromCall.Args, imp, importPath, diags, fset, "premise of infer.From")
		if !ok {
			return InferRule{}, false
		}
		return InferRule{From: premises, Given: contexts, To: conclusions}, true
	}
	return InferRule{}, false
}

// resolvePredicates resolves every arg as a list of Predicate leaves
// via resolveAndFlat — so each slot (From, Given, To) accepts bare
// predicates, pkg.Name selectors, and proven.And(...) decomposition.
// If any arg fails, a strict-mode diagnostic is already emitted at
// the innermost failing node and the whole slot is reported failed
// so the enclosing rule is dropped (a rule with a partially-resolved
// slot would quietly change semantics, which is exactly what strict
// mode rejects).
//
// Disjunctive obligations (proven.Or(...)) are NOT accepted in infer
// slots — a rule like "a OR b implies c" means two independent rules
// in flat terms, and splitting them at the scanner would obscure the
// declared shape. An Or here is rejected with a dedicated message
// pointing the user at declaring one rule per disjunct.
func resolvePredicates(args []ast.Expr, imp *importInfo, importPath string, diags *[]Diagnostic, fset *token.FileSet, role string) ([]Predicate, bool) {
	out := make([]Predicate, 0, len(args))
	anyBad := false
	for _, a := range args {
		leaves, ors, ok := resolveAndFlat(a, imp, importPath, fset, diags, role)
		if !ok {
			anyBad = true
			continue
		}
		if len(ors) > 0 {
			reportOrNotAcceptedInInfer(diags, fset, a, role)
			anyBad = true
			continue
		}
		out = append(out, leaves...)
	}
	if anyBad {
		return nil, false
	}
	return out, true
}

// reportOrNotAcceptedInInfer rejects a proven.Or appearing inside an
// infer.From / infer.Given / infer.To slot. A disjunction at a slot
// would synthesize multiple rules (one per branch) or need a
// disjunctive obligation/premise shape we do not carry on Rule yet;
// both are v2 scope.
func reportOrNotAcceptedInInfer(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: proven.Or is not accepted at %s — declare one rule per disjunct instead (infer.From(a).To(target) plus infer.From(b).To(target))",
			role,
		),
	})
}

// reportBadPredicate appends a strict-mode diagnostic pointing at
// expr's position with a standard "predicate must be a named
// function or pkg.Name selector" message. No-op when diags is nil.
// role is a short phrase naming the site (e.g. "argument to
// proven.That", "premise of infer.From") so the message tells the
// user *which* predicate slot is the problem when several appear
// close together.
func reportBadPredicate(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: %s must be a named function or pkg.Name selector — function literals and inline expressions are not supported; declare the predicate at package scope",
			role,
		),
	})
}

// reportBadSubject mirrors reportBadPredicate for the *subject* slot
// of proven.That: the value the obligation is asserted on must be a
// direct parameter identifier, since the discharge analyzer tracks
// facts by parameter index across call sites. Computed expressions
// (`proven.That(foo.bar, p)`, `proven.That(compute(x), p)`) are v2
// scope and fail the build in strict mode so users are not misled
// into thinking they've declared an obligation the preprocessor
// cannot actually enforce.
func reportBadSubject(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: %s must be a parameter identifier — computed subjects and non-parameter variables are not supported in v1",
			role,
		),
	})
}

// isInferIdent reports whether expr is the identifier for the
// import alias under which pkg/infer was imported.
func isInferIdent(expr ast.Expr, inferAlias string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == inferAlias
}

// importInfo holds the alias -> import-path mapping for a single
// file, plus a convenience pointer to the alias under which
// pkg/proven is imported (empty if not imported at all).
type importInfo struct {
	aliases     map[string]string
	provenAlias string
}

// collectImports builds the alias map for a file. Without type
// information we approximate the package name by the last path
// segment; that happens to be correct for every package we care
// about today (pkg/proven, and user predicate packages that follow
// Go's convention of package name = last segment). Packages that
// deviate (e.g. "gopkg.in/yaml.v3" → package "yaml") require an
// explicit import alias to participate, which is a documented
// limit of the scanner and cheaper to live with than linking in
// go/types for Phase 2.
func collectImports(f *ast.File) *importInfo {
	info := &importInfo{aliases: make(map[string]string)}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var alias string
		switch {
		case imp.Name != nil && (imp.Name.Name == "_" || imp.Name.Name == "."):
			continue
		case imp.Name != nil:
			alias = imp.Name.Name
		default:
			alias = lastPathSegment(path)
		}
		info.aliases[alias] = path
		if path == provenImportPath {
			info.provenAlias = alias
		}
	}
	return info
}

// scanFunc produces a FuncSummary for fn if and only if fn's body
// contains at least one proven.That or proven.Returns call whose
// arguments the scanner could resolve. Returns nil otherwise so
// unannotated functions do not clutter the summary map.
func scanFunc(fn *ast.FuncDecl, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic) *FuncSummary {
	summary := &FuncSummary{
		Name:       fn.Name.Name,
		ParamPreds: make(map[int][]Predicate),
	}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		summary.Recv = receiverTypeName(fn.Recv.List[0].Type)
	}
	paramIdx := buildParamIndex(fn.Type)
	trustAlias := imp.aliasFor(trustImportPath)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if kind, ok := matchProvenCall(call, imp.provenAlias); ok {
			switch kind {
			case "That":
				recordThat(summary, paramIdx, call, imp, importPath, fset, diags)
			case "Returns":
				recordReturns(summary, call, imp, importPath, fset, diags)
			}
			return true
		}
		// trust.Returns advertises the same function-level
		// postcondition shape as proven.Returns, just without
		// call-site verification. Record its predicates in
		// ReturnPreds so every caller downstream picks up the
		// fact via the cross-package sidecar path.
		if trustAlias != "" && isSel(call, trustAlias, "Returns") {
			recordReturns(summary, call, imp, importPath, fset, diags)
		}
		return true
	})

	if len(summary.ParamPreds) == 0 && len(summary.ReturnPreds) == 0 {
		return nil
	}
	return summary
}

// matchProvenCall reports whether call is of the form
// `<provenAlias>.That(...)` or `<provenAlias>.Returns(...)` and
// returns which one ("That" or "Returns").
func matchProvenCall(call *ast.CallExpr, provenAlias string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != provenAlias {
		return "", false
	}
	switch sel.Sel.Name {
	case "That", "Returns":
		return sel.Sel.Name, true
	}
	return "", false
}

// recordThat handles a matched proven.That call. The first
// argument must be a direct identifier naming a parameter of the
// enclosing function; a non-identifier subject or an identifier
// that isn't a parameter is a strict-mode error (complex subjects
// are v2 scope and cannot be tracked across call sites). Each
// subsequent argument must resolve to a named predicate; function
// literals, inline combinators, and other expressions fail the
// build rather than being silently dropped.
func recordThat(s *FuncSummary, paramIdx map[string]int, call *ast.CallExpr, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic) {
	if len(call.Args) < 2 {
		return
	}
	id, ok := call.Args[0].(*ast.Ident)
	if !ok {
		reportBadSubject(diags, fset, call.Args[0], "first argument to proven.That")
		return
	}
	idx, ok := paramIdx[id.Name]
	if !ok {
		reportBadSubject(diags, fset, call.Args[0], "first argument to proven.That")
		return
	}
	for _, arg := range call.Args[1:] {
		leaves, ors, ok := resolveAndFlat(arg, imp, importPath, fset, diags, "argument to proven.That")
		if !ok {
			continue
		}
		s.ParamPreds[idx] = append(s.ParamPreds[idx], leaves...)
		if len(ors) > 0 {
			if s.ParamOrs == nil {
				s.ParamOrs = make(map[int][][]Predicate)
			}
			s.ParamOrs[idx] = append(s.ParamOrs[idx], ors...)
		}
	}
}

// recordReturns handles a matched proven.Returns call. The value
// argument is not inspected by the scanner — any expression is
// permitted as a return value; the analyzer is responsible for
// verifying that the value actually satisfies the declared
// predicates. Each predicate argument must resolve to a named
// function or pkg.Name selector; function literals and inline
// combinator calls fail the build rather than being silently
// dropped.
func recordReturns(s *FuncSummary, call *ast.CallExpr, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic) {
	if len(call.Args) < 2 {
		return
	}
	for _, arg := range call.Args[1:] {
		leaves, ors, ok := resolveAndFlat(arg, imp, importPath, fset, diags, "argument to proven.Returns")
		if !ok {
			continue
		}
		s.ReturnPreds = append(s.ReturnPreds, leaves...)
		s.ReturnOrs = append(s.ReturnOrs, ors...)
	}
}

// resolvePredicate converts a predicate argument expression to a
// single Predicate identity. Supported shapes: a bare identifier
// (same-package predicate) and a package-qualified selector (an
// imported predicate). Combinators, function literals, and other
// expressions return (_, false) — callers that want to accept
// inline combinators must use resolveAndFlat instead. Used by the
// analyzer's guard walker, which planted facts are named by.
func resolvePredicate(expr ast.Expr, imp *importInfo, importPath string) (Predicate, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		return Predicate{Pkg: importPath, Name: e.Name}, true
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		if !ok {
			return Predicate{}, false
		}
		pkg, ok := imp.aliases[x.Name]
		if !ok {
			return Predicate{}, false
		}
		return Predicate{Pkg: pkg, Name: e.Sel.Name}, true
	}
	return Predicate{}, false
}

// resolveAndFlat is the obligation- and fact-site resolver. It
// accepts everything resolvePredicate does PLUS inline combinator
// calls under two rules:
//
//   - proven.And(...): arguments recurse through this function and
//     each resolved leaf / disjunction is concatenated. Nested And
//     flattens fully. proven.That(x, proven.And(a, b)) is stored as
//     two leaf obligations, identical to proven.That(x, a, b).
//   - proven.Or(...): arguments must each resolve to a single named
//     leaf — nested combinators inside Or are rejected by strict
//     mode. The whole Or becomes one entry in the returned `ors`
//     slice, representing a disjunctive obligation / fact that any
//     one alternative holding discharges.
//
// proven.Not is still rejected (it would need a negation-fact
// representation). Function literals and other unnamed values are
// still rejected. Any inner failure emits exactly one diagnostic at
// the innermost failing node.
//
// A degenerate proven.And() returns (nil, nil, true) — an AND of
// nothing is trivially true. proven.Or() with zero arguments is
// false and hence unsatisfiable; the scanner rejects it rather than
// silently storing an empty disjunction the caller can never
// discharge.
func resolveAndFlat(expr ast.Expr, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic, role string) (leaves []Predicate, ors [][]Predicate, ok bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		return []Predicate{{Pkg: importPath, Name: e.Name}}, nil, true
	case *ast.SelectorExpr:
		leaf, ok := selectorLeaf(e, imp)
		if !ok {
			reportBadPredicate(diags, fset, expr, role)
			return nil, nil, false
		}
		return []Predicate{leaf}, nil, true
	case *ast.CallExpr:
		op, isCombinator := provenCombinator(e, imp.provenAlias)
		if !isCombinator {
			reportBadPredicate(diags, fset, expr, role)
			return nil, nil, false
		}
		switch op {
		case "And":
			outLeaves := make([]Predicate, 0, len(e.Args))
			var outOrs [][]Predicate
			anyBad := false
			for _, arg := range e.Args {
				subLeaves, subOrs, ok := resolveAndFlat(arg, imp, importPath, fset, diags, role)
				if !ok {
					anyBad = true
					continue
				}
				outLeaves = append(outLeaves, subLeaves...)
				outOrs = append(outOrs, subOrs...)
			}
			if anyBad {
				return nil, nil, false
			}
			return outLeaves, outOrs, true
		case "Or":
			if len(e.Args) == 0 {
				reportEmptyOr(diags, fset, expr, role)
				return nil, nil, false
			}
			alts := make([]Predicate, 0, len(e.Args))
			anyBad := false
			for _, arg := range e.Args {
				leaf, ok := resolveLeafOnly(arg, imp, importPath, fset, diags, role)
				if !ok {
					anyBad = true
					continue
				}
				alts = append(alts, leaf)
			}
			if anyBad {
				return nil, nil, false
			}
			return nil, [][]Predicate{alts}, true
		case "Not":
			reportUnsupportedCombinator(diags, fset, expr, role, op)
			return nil, nil, false
		}
	}
	reportBadPredicate(diags, fset, expr, role)
	return nil, nil, false
}

// resolveLeafOnly accepts only a named leaf — Ident or pkg.Name
// selector. Used for proven.Or's arguments: nested combinators
// inside Or are v2 scope, so the scanner rejects them with the
// standard "must be a named function" diagnostic plus an Or-specific
// hint.
func resolveLeafOnly(expr ast.Expr, imp *importInfo, importPath string, fset *token.FileSet, diags *[]Diagnostic, role string) (Predicate, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		return Predicate{Pkg: importPath, Name: e.Name}, true
	case *ast.SelectorExpr:
		return selectorLeaf(e, imp)
	}
	reportOrArgNotLeaf(diags, fset, expr, role)
	return Predicate{}, false
}

// selectorLeaf resolves a pkg.Name selector to its Predicate
// identity, using the file's import-alias map to find the package
// path. Returns (_, false) if the selector's receiver is not an
// ident or that ident does not resolve to an imported package.
func selectorLeaf(e *ast.SelectorExpr, imp *importInfo) (Predicate, bool) {
	x, ok := e.X.(*ast.Ident)
	if !ok {
		return Predicate{}, false
	}
	pkg, ok := imp.aliases[x.Name]
	if !ok {
		return Predicate{}, false
	}
	return Predicate{Pkg: pkg, Name: e.Sel.Name}, true
}

// provenCombinator reports whether call is a <provenAlias>.<Op>(...)
// invocation where Op is one of And/Or/Not, returning the operator
// name. Any other shape returns ("", false).
func provenCombinator(call *ast.CallExpr, provenAlias string) (string, bool) {
	if provenAlias == "" {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != provenAlias {
		return "", false
	}
	switch sel.Sel.Name {
	case "And", "Or", "Not":
		return sel.Sel.Name, true
	}
	return "", false
}

// reportUnsupportedCombinator emits a strict-mode diagnostic for a
// proven.Not call appearing at an obligation or fact site. proven.And
// and proven.Or are accepted (And decomposes into leaf obligations,
// Or becomes a disjunctive obligation/fact); Not is v2 scope and
// fails the build with an explicit message so the user knows the cut.
func reportUnsupportedCombinator(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role, op string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: inline proven.%s at %s is not yet supported (proven.And and proven.Or are — Not needs a negation-fact representation the analyzer does not yet carry)",
			op, role,
		),
	})
}

// reportEmptyOr rejects proven.Or() with zero arguments. Semantically
// it is false and hence unsatisfiable; silently storing it as an empty
// disjunction would hand the caller an obligation they can never
// discharge, which is exactly what strict mode rejects.
func reportEmptyOr(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: empty proven.Or() at %s — a disjunction with no alternatives is unsatisfiable; supply at least one named predicate",
			role,
		),
	})
}

// reportOrArgNotLeaf rejects nested combinator calls (and function
// literals, etc.) inside a proven.Or's argument list. v1 only accepts
// proven.Or(leaf, leaf, ...) — mixing And or Or inside Or would
// require the preprocessor to reason about DNF/CNF normalization,
// which is v2 scope.
func reportOrArgNotLeaf(diags *[]Diagnostic, fset *token.FileSet, expr ast.Expr, role string) {
	if diags == nil || fset == nil {
		return
	}
	pos := fset.Position(expr.Pos())
	*diags = append(*diags, Diagnostic{
		File: pos.Filename,
		Line: pos.Line,
		Col:  pos.Column,
		Msg: fmt.Sprintf(
			"proven: arguments to proven.Or at %s must be named predicates — nested combinators inside Or are not supported; flatten manually or use inference rules",
			role,
		),
	})
}

// buildParamIndex maps each named parameter to its positional
// index. Fields without names (unusual in practice but legal, e.g.
// `func(_ int, _ string)`) advance the index without populating an
// entry.
func buildParamIndex(ft *ast.FuncType) map[string]int {
	idx := make(map[string]int)
	if ft.Params == nil {
		return idx
	}
	pos := 0
	for _, field := range ft.Params.List {
		if len(field.Names) == 0 {
			pos++
			continue
		}
		for _, name := range field.Names {
			if name.Name != "_" {
				idx[name.Name] = pos
			}
			pos++
		}
	}
	return idx
}

// receiverTypeName returns the named type of a method receiver,
// stripping a leading pointer if present. Generic receivers are
// currently flattened to the type's bare name (type parameters are
// ignored), which matches how Phase 3 will key lookups by type
// identity rather than instantiation.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	}
	return ""
}

func lastPathSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
