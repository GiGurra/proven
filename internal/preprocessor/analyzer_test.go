package preprocessor

import (
	"strings"
	"testing"
)

// Each sub-test mirrors one of the fixtures named in Phase 3 of
// docs/todo/roadmap.md: preceding_check_ok, unguarded_fails,
// early_return_guard_ok, returns_flow_ok, prove_then_proven_ok,
// prove_then_error_not_discharged_fails, plus the conjoined-&&
// source that the roadmap mentions among accepted fact shapes.

// pkgPath is the scanned source's own import path; used for
// same-package predicate identifiers.
const pkgPath = "example.com/ex"

// requireDischarge asserts that calls contains exactly one entry
// for calleeKey and that it has no Missing predicates on any param.
func requireDischarge(t *testing.T, calls []CallDischarge, calleeKey string) {
	t.Helper()
	c := dischargeForCallee(calls, calleeKey)
	if c == nil {
		t.Fatalf("no call to %q found; got %v", calleeKey, calls)
	}
	for _, p := range c.Params {
		if len(p.Missing) > 0 {
			t.Errorf("param %d of %s: missing %v (want discharged)", p.ParamIdx, calleeKey, p.Missing)
		}
	}
}

// requireMissing asserts that calls contains a call to calleeKey
// where param paramIdx is missing at least the given predicate
// name (same-package).
func requireMissing(t *testing.T, calls []CallDischarge, calleeKey string, paramIdx int, predName string) {
	t.Helper()
	c := dischargeForCallee(calls, calleeKey)
	if c == nil {
		t.Fatalf("no call to %q found; got %v", calleeKey, calls)
	}
	for _, p := range c.Params {
		if p.ParamIdx != paramIdx {
			continue
		}
		for _, m := range p.Missing {
			if m.Name == predName {
				return
			}
		}
		t.Errorf("param %d of %s: expected missing %q; got %v", paramIdx, calleeKey, predName, p.Missing)
		return
	}
	t.Errorf("param %d of %s not found in discharge %v", paramIdx, calleeKey, c.Params)
}

func TestAnalyze_PrecedingCheckDischarges(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isPositive(x) {
		target(x)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_UnguardedLeavesMissing(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isPositive")
}

func TestAnalyze_EarlyReturnGuardDischarges(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if !isPositive(x) {
		return
	}
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_EarlyPanicGuardDischarges(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if !isPositive(x) {
		panic("nope")
	}
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_ConjoinedGuardDischargesBoth(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }
func isSmall(x int) bool    { return x < 1000 }

func target(x int) {
	proven.That(x, isPositive)
	proven.That(x, isSmall)
}

func caller(x int) {
	if isPositive(x) && isSmall(x) {
		target(x)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_OnlyOneOfTwoLeavesMissing(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }
func isSmall(x int) bool    { return x < 1000 }

func target(x int) {
	proven.That(x, isPositive)
	proven.That(x, isSmall)
}

func caller(x int) {
	if isPositive(x) {
		target(x)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isSmall")
}

func TestAnalyze_ReturnsPostconditionFlowsForward(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func source() int { return proven.Returns(42, isPositive) }
func target(x int) { proven.That(x, isPositive) }

func caller() {
	v := source()
	target(v)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_ProveMustDischarges(t *testing.T) {
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/prove"
)

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(raw int) {
	v := prove.Must(raw, isPositive)
	target(v)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_ProveThatThenTargetDischarges(t *testing.T) {
	// prove.That's err-check guard is the canonical boundary
	// pattern. The analyzer records the fact on the value LHS at
	// the prove.That site; subsequent calls on that value
	// discharge the same predicate.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/prove"
)

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(raw int) error {
	v, err := prove.That(raw, isPositive)
	if err != nil {
		return err
	}
	target(v)
	return nil
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_ElseBranchEscapesDischarges(t *testing.T) {
	// if-else with the else-branch always escaping: the then-path
	// is the surviving continuation, so its facts persist after
	// the if-statement.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isPositive(x) {
		// fall through
	} else {
		panic("bad")
	}
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_UnrelatedPredicateDoesNotDischarge(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }
func isEven(x int) bool     { return x%2 == 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isEven(x) {
		target(x)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isPositive")
}

func TestAnalyze_FactOnDifferentVariableDoesNotDischarge(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(a, b int) {
	if isPositive(a) {
		target(b)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isPositive")
}

func TestAnalyze_DischargeIsScopedToThenBranch(t *testing.T) {
	// A preceding-check fact lives only inside the then-block; a
	// call after the if-statement is unguarded again.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isPositive(x) {
		// ok inside
	}
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isPositive")
}

// Integration-ish check: the analyzer and scanner agree on the
// Predicate identity so same-package predicate comparisons work
// end-to-end. If the two diverged (one stored pkg="", the other
// pkg=importPath), Has would miss and every test above would
// quietly fail. This test pins the invariant explicitly.
func TestAnalyze_PredicateIdentityMatchesScanner(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isPositive(x) {
		target(x)
	}
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	c := dischargeForCallee(calls, "target")
	if c == nil {
		t.Fatal("no discharge for target")
	}
	if len(c.Params) != 1 || len(c.Params[0].Required) != 1 {
		t.Fatalf("unexpected params: %v", c.Params)
	}
	req := c.Params[0].Required[0]
	if req.Pkg != pkgPath || req.Name != "isPositive" {
		t.Errorf("required predicate: got %+v, want {Pkg: %q, Name: %q}", req, pkgPath, "isPositive")
	}
	// Required must match stored facts exactly, otherwise Has
	// would fail. An empty Missing confirms that.
	if len(c.Params[0].Missing) != 0 {
		t.Errorf("expected no missing, got %v — predicate identity diverged between scanner and fact store", c.Params[0].Missing)
	}
	// Sanity: avoid a silent test-source typo breaking future
	// maintenance of this file.
	if !strings.Contains(src, "proven.That(x, isPositive)") {
		t.Fatal("test source changed shape; update assertions")
	}
}
