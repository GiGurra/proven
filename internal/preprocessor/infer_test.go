package preprocessor

import (
	"testing"
)

// Scanner-side tests for Phase 4: extracting InferRule entries
// from `var _ = infer.From(...).[Given(...).]To(...)` declarations.

func TestScan_InferRule_FromTo(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/infer"

func isSmallPositive(x int) bool { return x > 0 && x < 100 }
func isPositive(x int) bool      { return x > 0 }

var _ = infer.From(isSmallPositive).To(isPositive)
`
	sum := scanFromString(t, pkgPath, src)
	if len(sum.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d: %v", len(sum.Rules), sum.Rules)
	}
	r := sum.Rules[0]
	if r.From.Name != "isSmallPositive" || r.To.Name != "isPositive" {
		t.Errorf("rule shape wrong: %+v", r)
	}
	if r.Given != nil {
		t.Errorf("expected no Given, got %v", r.Given)
	}
}

func TestScan_InferRule_FromGivenTo(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/infer"

func isEven(x int) bool            { return x%2 == 0 }
func isGreaterThanZero(x int) bool { return x > 0 }
func isPositive(x int) bool        { return x > 0 }

var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
`
	sum := scanFromString(t, pkgPath, src)
	if len(sum.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(sum.Rules))
	}
	r := sum.Rules[0]
	if r.From.Name != "isEven" || r.To.Name != "isPositive" {
		t.Errorf("rule shape wrong: %+v", r)
	}
	if r.Given == nil || r.Given.Name != "isGreaterThanZero" {
		t.Errorf("Given predicate missing/wrong: %v", r.Given)
	}
}

func TestScan_InferRule_CrossPackagePredicate(t *testing.T) {
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/infer"
	"example.com/preds"
)

func isLocalSmall(x int) bool { return x < 100 }

var _ = infer.From(isLocalSmall).To(preds.IsBounded)
`
	sum := scanFromString(t, pkgPath, src)
	if len(sum.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(sum.Rules))
	}
	r := sum.Rules[0]
	if r.From != (Predicate{Pkg: pkgPath, Name: "isLocalSmall"}) {
		t.Errorf("From wrong: %+v", r.From)
	}
	if r.To != (Predicate{Pkg: "example.com/preds", Name: "IsBounded"}) {
		t.Errorf("To wrong: %+v", r.To)
	}
}

func TestScan_InferRule_MultipleRules(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/infer"

func a(int) bool { return true }
func b(int) bool { return true }
func c(int) bool { return true }
func d(int) bool { return true }

var (
	_ = infer.From(a).To(b)
	_ = infer.From(b).To(c)
	_ = infer.From(c).Given(d).To(a)
)
`
	sum := scanFromString(t, pkgPath, src)
	if len(sum.Rules) != 3 {
		t.Fatalf("want 3 rules, got %d: %v", len(sum.Rules), sum.Rules)
	}
}

func TestScan_InferRule_PlainVarDeclIgnored(t *testing.T) {
	// A var decl that does not match the fluent shape must not
	// produce a rule entry.
	src := `package ex

import "github.com/GiGurra/proven/pkg/infer"

func a(int) bool { return true }

var _ = 42
var notARule = infer.From(a) // incomplete chain — no .To
var _ = a
`
	sum := scanFromString(t, pkgPath, src)
	if len(sum.Rules) != 0 {
		t.Errorf("want 0 rules for non-matching declarations, got %v", sum.Rules)
	}
}

// Analyzer-side tests: using rules in discharge checks.

func TestAnalyze_InferRule_DischargesBySubsumption(t *testing.T) {
	// Fact established: isSmallPositive(x). Rule: isSmallPositive
	// ⇒ isPositive. Callee requires isPositive. Discharge via
	// implication.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func isSmallPositive(x int) bool { return x > 0 && x < 100 }
func isPositive(x int) bool      { return x > 0 }

var _ = infer.From(isSmallPositive).To(isPositive)

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isSmallPositive(x) {
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

func TestAnalyze_InferRule_GivenContextRequired(t *testing.T) {
	// Rule: isEven ⇒ isPositive, GIVEN isGreaterThanZero. Facts
	// must establish BOTH isEven AND isGreaterThanZero on x for
	// discharge to succeed. Only isEven established → still
	// missing isPositive.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func isEven(x int) bool            { return x%2 == 0 }
func isGreaterThanZero(x int) bool { return x > 0 }
func isPositive(x int) bool        { return x > 0 }

var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

func target(x int) { proven.That(x, isPositive) }

func callerPartial(x int) {
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

func TestAnalyze_InferRule_GivenContextSatisfied(t *testing.T) {
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func isEven(x int) bool            { return x%2 == 0 }
func isGreaterThanZero(x int) bool { return x > 0 }
func isPositive(x int) bool        { return x > 0 }

var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isEven(x) && isGreaterThanZero(x) {
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

func TestAnalyze_InferRule_ChainedImplications(t *testing.T) {
	// A ⇒ B ⇒ C: fact A on x, target requires C → discharge
	// through two hops.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func predA(x int) bool { return true }
func predB(x int) bool { return true }
func predC(x int) bool { return true }

var _ = infer.From(predA).To(predB)
var _ = infer.From(predB).To(predC)

func target(x int) { proven.That(x, predC) }

func caller(x int) {
	if predA(x) {
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

func TestAnalyze_InferRule_MissingRuleStillFails(t *testing.T) {
	// No rule connecting isSmallPositive to isPositive; the fact
	// does not discharge the requirement by implication.
	src := `package ex

import "github.com/GiGurra/proven/pkg/proven"

func isSmallPositive(x int) bool { return x > 0 && x < 100 }
func isPositive(x int) bool      { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(x int) {
	if isSmallPositive(x) {
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

func TestAnalyze_InferRule_CyclicRulesDoNotHang(t *testing.T) {
	// A ⇒ B and B ⇒ A with no facts → discharge returns false
	// without recursing forever. Sentinel: the test returning at
	// all is the pass.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func predA(x int) bool { return true }
func predB(x int) bool { return true }

var _ = infer.From(predA).To(predB)
var _ = infer.From(predB).To(predA)

func target(x int) { proven.That(x, predA) }

func caller(x int) {
	target(x)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "predA")
}

func TestAnalyze_InferRule_ChainedThroughGiven(t *testing.T) {
	// Rule X: A ⇒ M.
	// Rule Y: M, Given(C) ⇒ T.
	// Facts on x: A(x), C(x). Target requires T. The chain must
	// prove M via rule X, then T via rule Y with C satisfied.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/infer"
)

func predA(x int) bool { return true }
func predM(x int) bool { return true }
func predC(x int) bool { return true }
func predT(x int) bool { return true }

var _ = infer.From(predA).To(predM)
var _ = infer.From(predM).Given(predC).To(predT)

func target(x int) { proven.That(x, predT) }

func caller(x int) {
	if predA(x) && predC(x) {
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
