package preprocessor

import (
	"strings"
	"testing"
)

// trust.That is treated by the analyzer as a fact injection,
// parallel to prove.Must but without a runtime check — the
// preprocessor trusts the programmer's assertion and propagates
// it downstream.

func TestAnalyze_TrustThatInjectsFact(t *testing.T) {
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(raw int) {
	v := trust.That(raw, isPositive)
	target(v)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireDischarge(t, calls, "target")
}

func TestAnalyze_TrustThatDoesNotCoverOtherPredicate(t *testing.T) {
	// Asserting isEven via trust.That does not satisfy an
	// isPositive obligation. Trust only injects the predicates
	// it is given; it is not a universal discharge.
	src := `package ex

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

func isPositive(x int) bool { return x > 0 }
func isEven(x int) bool     { return x%2 == 0 }

func target(x int) { proven.That(x, isPositive) }

func caller(raw int) {
	v := trust.That(raw, isEven)
	target(v)
}
`
	calls, err := analyzeSource(pkgPath, src)
	if err != nil {
		t.Fatal(err)
	}
	requireMissing(t, calls, "target", 0, "isPositive")
}

func TestRewrite_TrustThatIsErased(t *testing.T) {
	src := `package ex

import "github.com/GiGurra/proven/pkg/trust"

func isPositive(x int) bool { return x > 0 }

func caller(raw int) int {
	v := trust.That(raw, isPositive)
	return v
}
`
	out, changed := rewriteString(t, src)
	if !changed {
		t.Fatal("expected rewrite")
	}
	s := string(out)
	// Assert the call site is erased. An appended sentinel
	// (`var _ = trust.That[struct{}]`) keeps the import alive,
	// so the literal substring `trust.That` may still appear at
	// the very end; we check the caller body specifically.
	prefix := strings.Split(s, "var _ = ")[0]
	if strings.Contains(prefix, "trust.That") {
		t.Errorf("trust.That still present in caller body:\n%s", s)
	}
	if !strings.Contains(prefix, "raw") {
		t.Errorf("inner value identifier lost:\n%s", s)
	}
	if !parsesCleanly(t, out) {
		t.Errorf("rewritten source does not parse:\n%s", s)
	}
}
