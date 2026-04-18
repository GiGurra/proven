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
// expressions"). An unresolvable predicate expression (inline
// combinator, arbitrary expression, function literal) is silently
// skipped today; Phase 3 will decide how to surface that as an
// undischargeable obligation.

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

// PackageSummary is the scanner output for one compile unit. Only
// functions with at least one obligation are present in Funcs.
type PackageSummary struct {
	ImportPath string
	Funcs      map[string]*FuncSummary
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
		scanFile(sum, importPath, f)
	}
	return sum, nil
}

// scanFile walks one parsed file's top-level FuncDecls, collecting
// any obligations into sum. Files that do not import pkg/proven
// produce no entries and are skipped early.
func scanFile(sum *PackageSummary, importPath string, f *ast.File) {
	imp := collectImports(f)
	if imp.provenAlias == "" {
		return
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		summary := scanFunc(fn, imp, importPath)
		if summary == nil {
			continue
		}
		sum.Funcs[summary.Key()] = summary
	}
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
func scanFunc(fn *ast.FuncDecl, imp *importInfo, importPath string) *FuncSummary {
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
			recordThat(summary, paramIdx, call, imp, importPath)
		case "Returns":
			recordReturns(summary, call, imp, importPath)
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
// enclosing function — other shapes are v2 scope and are ignored.
// Subsequent arguments are each resolved to a Predicate.
func recordThat(s *FuncSummary, paramIdx map[string]int, call *ast.CallExpr, imp *importInfo, importPath string) {
	if len(call.Args) < 2 {
		return
	}
	id, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return
	}
	idx, ok := paramIdx[id.Name]
	if !ok {
		return
	}
	for _, arg := range call.Args[1:] {
		if pred, ok := resolvePredicate(arg, imp, importPath); ok {
			s.ParamPreds[idx] = append(s.ParamPreds[idx], pred)
		}
	}
}

// recordReturns handles a matched proven.Returns call. The value
// argument is not inspected (any expression is permitted as a
// return value); only the predicates are recorded.
func recordReturns(s *FuncSummary, call *ast.CallExpr, imp *importInfo, importPath string) {
	if len(call.Args) < 2 {
		return
	}
	for _, arg := range call.Args[1:] {
		if pred, ok := resolvePredicate(arg, imp, importPath); ok {
			s.ReturnPreds = append(s.ReturnPreds, pred)
		}
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
