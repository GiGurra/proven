package infertest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/infertest"
)

func isPositive(x int) bool        { return x > 0 }
func isSmallPositive(x int) bool   { return x > 0 && x < 100 }
func isEven(x int) bool            { return x%2 == 0 }
func isGreaterThanZero(x int) bool { return x > 0 }

// TestVerify_SoundRulePasses — a correct implication passes Verify
// with no failure reports, across both premise-hitting samples and
// vacuous premise-miss samples.
func TestVerify_SoundRulePasses(t *testing.T) {
	rule := infer.From(isSmallPositive).To(isPositive)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 1, 5, 50, 99, -3, 0)
	if rec.failed {
		t.Errorf("sound rule reported failure: %s", rec.log.String())
	}
}

// TestVerify_UnsoundRuleFails — isEven does not imply isPositive
// (counter-example: -4 is even but not positive). Verify must report
// it.
func TestVerify_UnsoundRuleFails(t *testing.T) {
	rule := infer.From(isEven).To(isPositive)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 2, 4, -4, -6)
	if !rec.failed {
		t.Fatal("unsound rule did not report any failure")
	}
	msg := rec.log.String()
	if !strings.Contains(msg, "rule violated on sample -4") {
		t.Errorf("expected counter-example -4 in failure message, got: %s", msg)
	}
}

// TestVerify_GivenRulePasses — a rule with a Given clause is vacuously
// satisfied when Given does not hold, and genuinely satisfied when
// Given + premise both hold and conclusion follows.
func TestVerify_GivenRulePasses(t *testing.T) {
	// isEven ∧ isGreaterThanZero ⇒ isPositive  (holds)
	rule := infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 2, 4, 100, -2 /* Given false — skipped */, -4 /* Given false — skipped */)
	if rec.failed {
		t.Errorf("sound Given rule reported failure: %s", rec.log.String())
	}
}

// TestVerify_GivenRuleCatchesFailure — a false Given rule still gets
// caught when premise + Given are both satisfied.
func TestVerify_GivenRuleCatchesFailure(t *testing.T) {
	// isPositive ∧ isEven ⇒ isSmallPositive  (fails at 100 — isPositive
	// and isEven hold, but isSmallPositive does not because x < 100).
	rule := infer.From(isPositive).Given(isEven).To(isSmallPositive)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 2, 10, 100)
	if !rec.failed {
		t.Fatal("unsound Given rule did not report failure")
	}
	if !strings.Contains(rec.log.String(), "rule violated on sample 100") {
		t.Errorf("expected counter-example 100, got: %s", rec.log.String())
	}
}

// TestVerify_AllVacuousIsSilent — Verify makes no claim about samples
// whose premise does not hold. A sample set that entirely misses the
// premise passes silently. (VerifyApplies is the opt-in stricter
// variant.)
func TestVerify_AllVacuousIsSilent(t *testing.T) {
	rule := infer.From(isSmallPositive).To(isPositive)
	rec := &recordingT{}
	infertest.Verify(rec, rule, -1, -2, -3)
	if rec.failed {
		t.Errorf("vacuous sample set reported failure: %s", rec.log.String())
	}
}

// TestVerifyApplies_FailsOnAllVacuous — VerifyApplies refuses to
// silently pass on a sample set that never hits the premise.
func TestVerifyApplies_FailsOnAllVacuous(t *testing.T) {
	rule := infer.From(isSmallPositive).To(isPositive)
	rec := &recordingT{}
	infertest.VerifyApplies(rec, rule, -1, -2, -3)
	if !rec.failed {
		t.Fatal("VerifyApplies did not fail on an all-vacuous sample set")
	}
	if !strings.Contains(rec.log.String(), "no sample satisfied the rule's premise") {
		t.Errorf("expected 'no sample satisfied' message, got: %s", rec.log.String())
	}
}

// TestVerifyApplies_PassesWhenSomeHit — with at least one premise-
// hitting sample, VerifyApplies passes the same as Verify.
func TestVerifyApplies_PassesWhenSomeHit(t *testing.T) {
	rule := infer.From(isSmallPositive).To(isPositive)
	rec := &recordingT{}
	infertest.VerifyApplies(rec, rule, -1, 5, -2)
	if rec.failed {
		t.Errorf("VerifyApplies reported failure on a well-covered sample set: %s", rec.log.String())
	}
}

// TestVerify_MultiPremiseAndMultiConclusion — variadic From and To
// slots AND-compose. The rule below reads "isEven AND isPositive
// implies isSmallPositive AND isGreaterThanZero", and holds for
// every positive even under 100. -2 misses the premise (isPositive
// is false) so Verify skips it; 4 satisfies both conclusions.
func TestVerify_MultiPremiseAndMultiConclusion(t *testing.T) {
	rule := infer.From(isEven, isPositive).To(isSmallPositive, isGreaterThanZero)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 2, 4, 98, -2, -4)
	if rec.failed {
		t.Errorf("sound multi-premise / multi-conclusion rule reported failure: %s", rec.log.String())
	}
}

// TestVerify_MultiConclusionCatchesAnyFailure — with two conclusions,
// a sample that satisfies the premise but violates either conclusion
// is still a counter-example. isPositive AND isEven does not imply
// isSmallPositive AND isGreaterThanZero: 100 violates isSmallPositive
// even though isGreaterThanZero holds.
func TestVerify_MultiConclusionCatchesAnyFailure(t *testing.T) {
	rule := infer.From(isPositive, isEven).To(isSmallPositive, isGreaterThanZero)
	rec := &recordingT{}
	infertest.Verify(rec, rule, 4, 100)
	if !rec.failed {
		t.Fatal("multi-conclusion rule did not catch a per-conclusion counter-example")
	}
	if !strings.Contains(rec.log.String(), "rule violated on sample 100") {
		t.Errorf("expected counter-example 100, got: %s", rec.log.String())
	}
}

// recordingT captures Errorf calls so tests can assert on Verify's
// behavior without actually failing the enclosing test. Implements
// infertest.TestingT.
type recordingT struct {
	failed bool
	log    strings.Builder
}

func (r *recordingT) Helper() {}

func (r *recordingT) Errorf(format string, args ...any) {
	r.failed = true
	fmt.Fprintf(&r.log, format, args...)
	r.log.WriteByte('\n')
}
