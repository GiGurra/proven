package preprocessor

// Phase 1 handler: augment pkg/proven's compile with a synthesized
// Go source file that supplies the _proven_atCompileTime linker
// symbol as a no-op. Without this, any program that imports
// pkg/proven fails to link — by design; the absence of the symbol is
// what makes forgetting the preprocessor a loud, deterministic error.
//
// The stub's signature is derived from the AST of the package itself:
// we parse the source files received in the compile's argv, find the
// //go:linkname directive that binds a local func to
// _proven_atCompileTime, and shape the stub to match. A hardcoded
// template would work today, but the AST path lets future API
// evolution in pkg/proven (e.g. adding a return value or changing
// the closure shape) drive the stub mechanically rather than
// requiring a paired edit to the preprocessor.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
)

// provenPkgPath is the package whose compile gets the stub.
const provenPkgPath = "github.com/GiGurra/proven/pkg/proven"

// provenLinkSymbol is the linker name declared via //go:linkname in
// pkg/proven, and therefore the symbol our stub must supply.
const provenLinkSymbol = "_proven_atCompileTime"

// planProvenStub walks the pkg/proven source files the compiler was
// handed, finds the //go:linkname declaration binding a local func
// to _proven_atCompileTime, and writes a temp Go file that provides
// that symbol as a no-op. The returned path is appended to the
// compile argv by the caller.
//
// If no matching declaration is present the function returns
// (nil, nil, nil) — the package in question is not what we expect,
// and silently doing nothing leaves the link failing as it would
// without the preprocessor, which is an intelligible failure mode.
func planProvenStub(sources []string) ([]string, func(), error) {
	fset := token.NewFileSet()
	decl, err := findLinknameDecl(fset, sources, provenLinkSymbol)
	if err != nil {
		return nil, nil, err
	}
	if decl == nil {
		return nil, nil, nil
	}

	stub, err := renderStub(decl)
	if err != nil {
		return nil, nil, err
	}

	path, cleanup, err := writeTempGoFile("proven-stub-*.go", stub)
	if err != nil {
		return nil, nil, err
	}
	return []string{path}, cleanup, nil
}

// linknameDecl captures the essentials of a //go:linkname-bound
// forward declaration: the enclosing file's package name and the
// function's type, re-used verbatim when we synthesize the stub so
// the emitted symbol has a matching signature.
type linknameDecl struct {
	pkgName string
	fnType  *ast.FuncType
}

// findLinknameDecl scans each source file for a //go:linkname
// directive whose target symbol matches `symbol`, bound to a
// function declaration with no body. Returns the matched decl's
// metadata or nil if no match is found.
func findLinknameDecl(fset *token.FileSet, sources []string, symbol string) (*linknameDecl, error) {
	for _, src := range sources {
		decl, err := scanFileForLinkname(fset, src, symbol)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", src, err)
		}
		if decl != nil {
			return decl, nil
		}
	}
	return nil, nil
}

// scanFileForLinkname parses one source file and searches its
// top-level declarations for a //go:linkname comment binding a
// bodiless function to `symbol`.
func scanFileForLinkname(fset *token.FileSet, path, symbol string) (*linknameDecl, error) {
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body != nil || fn.Doc == nil {
			continue
		}
		if !linknameMatches(fn.Doc, fn.Name.Name, symbol) {
			continue
		}
		return &linknameDecl{
			pkgName: f.Name.Name,
			fnType:  fn.Type,
		}, nil
	}
	return nil, nil
}

// linknameMatches reports whether any //go:linkname comment in the
// doc group binds the named local function to `symbol`.
func linknameMatches(doc *ast.CommentGroup, localName, symbol string) bool {
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if !strings.HasPrefix(text, "go:linkname ") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 3 {
			continue
		}
		if fields[1] == localName && fields[2] == symbol {
			return true
		}
	}
	return false
}

// renderStub turns a discovered linkname declaration into the full
// source text of a companion Go file that provides the bound symbol
// as a no-op. The stub's function signature is copied from the
// original FuncType so the two are guaranteed type-compatible —
// changes to the pkg/proven signature flow through mechanically.
func renderStub(d *linknameDecl) ([]byte, error) {
	stubName := "provenAtCompileTimeImpl"

	fset := token.NewFileSet()
	fnDecl := &ast.FuncDecl{
		Name: ast.NewIdent(stubName),
		Type: d.fnType,
		Body: &ast.BlockStmt{},
	}

	var sig bytes.Buffer
	if err := printer.Fprint(&sig, fset, fnDecl); err != nil {
		return nil, fmt.Errorf("print stub signature: %w", err)
	}

	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by the proven preprocessor. DO NOT EDIT.")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "package %s\n\n", d.pkgName)
	fmt.Fprintln(&out, `import _ "unsafe"`)
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "//go:linkname %s %s\n", stubName, provenLinkSymbol)
	out.Write(sig.Bytes())
	fmt.Fprintln(&out)
	return out.Bytes(), nil
}

// writeTempGoFile writes content to a uniquely-named temp file
// matching the given pattern and returns its path with a cleanup
// that deletes it.
func writeTempGoFile(pattern string, content []byte) (string, func(), error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	path := f.Name()
	return path, func() { os.Remove(path) }, nil
}
