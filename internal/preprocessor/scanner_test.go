package preprocessor

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// scanFromString is a test helper that writes src to a temp file
// and runs ScanPackage on it. importPath is used as the scanned
// package's own import path.
func scanFromString(t *testing.T, importPath, src string) *PackageSummary {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "input.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := ScanPackage(importPath, []string{path})
	if err != nil {
		t.Fatalf("ScanPackage: %v", err)
	}
	return sum
}

// sortedPreds returns p sorted by (Pkg, Name) so test assertions
// don't depend on the order AST walks produced them in. All tests
// use sets rather than lists for predicate assertions.
func sortedPreds(p []Predicate) []Predicate {
	out := append([]Predicate(nil), p...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pkg != out[j].Pkg {
			return out[i].Pkg < out[j].Pkg
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func TestScan_SingleParamSinglePredicate(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func Transfer(amount int) error {
	proven.That(amount, isPositive)
	return nil
}
`
	sum := scanFromString(t, "example.com/ex", src)

	fn, ok := sum.Funcs["Transfer"]
	if !ok {
		t.Fatalf("Transfer missing; got %v", sum.Funcs)
	}
	want := map[int][]Predicate{
		0: {{Pkg: "example.com/ex", Name: "isPositive"}},
	}
	if !reflect.DeepEqual(fn.ParamPreds, want) {
		t.Errorf("ParamPreds: got %v, want %v", fn.ParamPreds, want)
	}
}

func TestScan_MultiplePredicatesPerParam(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isNonEmpty(s string) bool { return len(s) > 0 }
func maxLen280(s string) bool  { return len(s) <= 280 }

func Post(note string) {
	proven.That(note, isNonEmpty, maxLen280)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	got := sortedPreds(sum.Funcs["Post"].ParamPreds[0])
	want := []Predicate{
		{Pkg: "example.com/ex", Name: "isNonEmpty"},
		{Pkg: "example.com/ex", Name: "maxLen280"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScan_MultipleParams(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool    { return x > 0 }
func isNonEmpty(s string) bool { return len(s) > 0 }

func Transfer(amount int, note string) {
	proven.That(amount, isPositive)
	proven.That(note, isNonEmpty)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	params := sum.Funcs["Transfer"].ParamPreds
	if len(params) != 2 {
		t.Fatalf("want 2 param entries, got %d: %v", len(params), params)
	}
	if params[0][0].Name != "isPositive" {
		t.Errorf("param 0: got %v, want isPositive", params[0][0])
	}
	if params[1][0].Name != "isNonEmpty" {
		t.Errorf("param 1: got %v, want isNonEmpty", params[1][0])
	}
}

func TestScan_CrossPackagePredicate(t *testing.T) {
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"example.com/preds"
)

func Do(amount int) {
	proven.That(amount, preds.IsPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	preds := sum.Funcs["Do"].ParamPreds[0]
	want := Predicate{Pkg: "example.com/preds", Name: "IsPositive"}
	if len(preds) != 1 || preds[0] != want {
		t.Errorf("got %v, want [%v]", preds, want)
	}
}

func TestScan_ReturnsPostcondition(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func FindUserID() int {
	return proven.Returns(42, isPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	got := sum.Funcs["FindUserID"].ReturnPreds
	want := []Predicate{{Pkg: "example.com/ex", Name: "isPositive"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScan_AliasedProvenImport(t *testing.T) {
	src := `package ex

import p "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func Transfer(amount int) {
	p.That(amount, isPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	if sum.Funcs["Transfer"] == nil {
		t.Fatal("Transfer missing; aliased proven import was not detected")
	}
	if sum.Funcs["Transfer"].ParamPreds[0][0].Name != "isPositive" {
		t.Errorf("got %v, want isPositive", sum.Funcs["Transfer"].ParamPreds[0])
	}
}

func TestScan_MethodWithReceiver(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

type Account struct{ Balance int }

func isPositive(x int) bool { return x > 0 }

func (a *Account) Credit(amount int) {
	proven.That(amount, isPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	fn, ok := sum.Funcs["Account.Credit"]
	if !ok {
		t.Fatalf("Account.Credit missing; got %v", sum.Funcs)
	}
	if fn.Recv != "Account" {
		t.Errorf("Recv: got %q, want %q", fn.Recv, "Account")
	}
}

func TestScan_FunctionWithoutProven(t *testing.T) {
	src := `package ex

func Plain(x int) int { return x * 2 }
`
	sum := scanFromString(t, "example.com/ex", src)
	if len(sum.Funcs) != 0 {
		t.Errorf("want empty Funcs, got %v", sum.Funcs)
	}
}

func TestScan_SkipsUnresolvedPredicateExpressions(t *testing.T) {
	// When the scanner is called with diags == nil (stand-alone
	// API path, as scanFromString does below), an unresolvable
	// predicate expression — combinator call, function literal,
	// arbitrary expression — is silently dropped from the summary.
	// The production toolexec path passes a non-nil diags and
	// turns the same shape into a build-failing diagnostic; see
	// testdata/cases/predicate_lambda_fails and
	// predicate_inline_combinator_fails for that path. This test
	// locks in the lenient-mode behavior specifically so the
	// stand-alone API can keep scanning partially-broken sources
	// without panicking.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }
func isSmall(x int) bool    { return x < 1000 }

func Do(amount int) {
	proven.That(amount, proven.And(isPositive, isSmall))
	proven.That(amount, func(x int) bool { return x > 0 })
	proven.That(amount, isPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	preds := sum.Funcs["Do"].ParamPreds[0]
	// Only the bare ident isPositive should be recorded.
	want := []Predicate{{Pkg: "example.com/ex", Name: "isPositive"}}
	if !reflect.DeepEqual(preds, want) {
		t.Errorf("got %v, want %v", preds, want)
	}
}

func TestScan_NonIdentValueArgIsSkipped(t *testing.T) {
	// The first argument to proven.That must be a direct parameter
	// reference in v1. In lenient mode (diags == nil, as used by
	// this test's scanFromString helper) calls with any other value
	// expression produce no obligation entry — Phase 3 would not be
	// able to tie the obligation to any specific parameter anyway.
	// In strict mode (diags != nil, the production toolexec path) a
	// non-parameter subject fails the build; see
	// testdata/cases/that_non_param_subject_fails.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func Do(amount int) {
	proven.That(amount+1, isPositive)
	proven.That(42, isPositive)
}
`
	sum := scanFromString(t, "example.com/ex", src)
	if fn := sum.Funcs["Do"]; fn != nil {
		t.Errorf("want no summary (all calls have non-ident values); got %v", fn)
	}
}

func TestScan_MultipleFilesSamePackage(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte(`package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func A(x int) { proven.That(x, isPositive) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(`package ex

import "github.com/GiGurra/proven/pkg/proven"

func isNonEmpty(s string) bool { return len(s) > 0 }

func B(s string) { proven.That(s, isNonEmpty) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := ScanPackage("example.com/ex", []string{fileA, fileB})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Funcs["A"] == nil || sum.Funcs["B"] == nil {
		t.Errorf("want both A and B; got %v", sum.Funcs)
	}
}
