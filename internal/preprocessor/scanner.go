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
// list of predicates asserted on that parameter by every
// proven.That call in the body. An entry is present only if at
// least one such call exists; absent parameters carry no
// obligations.
//
// ReturnPreds: predicates collected across every proven.Returns
// call in the body. Phase 2 treats these as a bag because tracking
// "which return value position is constrained" would require flow
// analysis over the body, which is Phase 3's job. A function that
// uses proven.Returns in one return statement and not another is
// represented by a single combined list; the ambiguity is
// acceptable at this level because Phase 3 will re-walk the body
// with call-site context anyway.
//
// Recv is the receiver type identifier for methods (without the
// leading star), empty for free functions.
type FuncSummary struct {
	Name        string
	Recv        string
	ParamPreds  map[int][]Predicate
	ReturnPreds []Predicate
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
// harvested from `var _ = infer.From(premise).[Given(context).]To(conclusion)`.
// Given is non-nil only when a .Given(...) step was present.
//
// Rules are trusted — the scanner does not verify that the
// implication actually holds (docs/design.md). Unresolvable
// predicate arguments (combinator calls, function literals, etc.)
// cause the rule to be skipped rather than silently stored with
// an empty identity.
type InferRule struct {
	From  Predicate
	Given *Predicate
	To    Predicate
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
	if imp.provenAlias == "" && inferAlias == "" {
		return
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil || imp.provenAlias == "" {
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
//	infer.From(premise).To(conclusion)
//	infer.From(premise).Given(context).To(conclusion)
//
// and returns the rule identity. The outermost call is always
// `.To(conclusion)`; its receiver is either `.From(premise)`
// directly or `.Given(context)` wrapping `.From(premise)`.
// Structural mismatches (a call that does not match either fluent
// shape at all) silently return (_, false) — they are not rules.
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
	if !ok || toSel.Sel.Name != "To" || len(toCall.Args) != 1 {
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
	conclusion, ok := resolvePredicate(toCall.Args[0], imp, importPath)
	if !ok {
		reportBadPredicate(diags, fset, toCall.Args[0], "conclusion of infer rule")
		return InferRule{}, false
	}

	switch innerSel.Sel.Name {
	case "From":
		// infer.From(premise).To(conclusion)
		if !isInferIdent(innerSel.X, inferAlias) {
			return InferRule{}, false
		}
		if len(inner.Args) != 1 {
			return InferRule{}, false
		}
		premise, ok := resolvePredicate(inner.Args[0], imp, importPath)
		if !ok {
			reportBadPredicate(diags, fset, inner.Args[0], "premise of infer.From")
			return InferRule{}, false
		}
		return InferRule{From: premise, To: conclusion}, true

	case "Given":
		// infer.From(premise).Given(context).To(conclusion)
		if len(inner.Args) != 1 {
			return InferRule{}, false
		}
		fromCall, ok := innerSel.X.(*ast.CallExpr)
		if !ok {
			return InferRule{}, false
		}
		fromSel, ok := fromCall.Fun.(*ast.SelectorExpr)
		if !ok || fromSel.Sel.Name != "From" || len(fromCall.Args) != 1 {
			return InferRule{}, false
		}
		if !isInferIdent(fromSel.X, inferAlias) {
			return InferRule{}, false
		}
		given, ok := resolvePredicate(inner.Args[0], imp, importPath)
		if !ok {
			reportBadPredicate(diags, fset, inner.Args[0], "context of infer.Given")
			return InferRule{}, false
		}
		premise, ok := resolvePredicate(fromCall.Args[0], imp, importPath)
		if !ok {
			reportBadPredicate(diags, fset, fromCall.Args[0], "premise of infer.From")
			return InferRule{}, false
		}
		return InferRule{From: premise, Given: &given, To: conclusion}, true
	}
	return InferRule{}, false
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

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		kind, ok := matchProvenCall(call, imp.provenAlias)
		if !ok {
			return true
		}
		switch kind {
		case "That":
			recordThat(summary, paramIdx, call, imp, importPath, fset, diags)
		case "Returns":
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
		pred, ok := resolvePredicate(arg, imp, importPath)
		if !ok {
			reportBadPredicate(diags, fset, arg, "argument to proven.That")
			continue
		}
		s.ParamPreds[idx] = append(s.ParamPreds[idx], pred)
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
		pred, ok := resolvePredicate(arg, imp, importPath)
		if !ok {
			reportBadPredicate(diags, fset, arg, "argument to proven.Returns")
			continue
		}
		s.ReturnPreds = append(s.ReturnPreds, pred)
	}
}

// resolvePredicate converts a predicate argument expression to a
// Predicate identity. Supported shapes: a bare identifier (same-
// package predicate) and a package-qualified selector (an imported
// predicate). Combinators (proven.And(...)), function literals,
// and arbitrary expressions return (_, false); the caller skips
// them.
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
