// Package infertest property-tests declared infer rules against
// sample inputs to catch declared-but-false implications.
//
// Inference rules in infer are trusted — the proven preprocessor uses
// them to discharge obligations by subsumption without symbolically
// verifying that the declared implication actually holds. A rule like
// infer.From(isEven).To(isPositive) would silently hand out discharges
// for false conclusions. infertest.Verify offers a cheap runtime
// check: run the rule on a batch of sample values and fail the test
// whenever the premise (and Given, if present) is satisfied but the
// conclusion is not.
//
// Verify is a property-style check, not a proof: it covers only the
// samples you supply. A rule that holds on the samples may still be
// false on an uncovered input. Treat Verify as a way to catch obvious
// mistakes and typos, not as a soundness oracle.
//
// Usage:
//
//	var ruleSmallPositiveIsPositive = infer.From(isSmallPositive).To(isPositive)
//
//	func TestRule(t *testing.T) {
//	    infertest.Verify(t, ruleSmallPositiveIsPositive, 1, 5, 99)
//	}
package infertest

import "github.com/GiGurra/proven/pkg/infer"

// TestingT is the minimal subset of *testing.T that Verify uses. Any
// *testing.T satisfies it; a fake recording implementation can be
// substituted in infertest's own tests.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// Verify runs rule against each sample and reports any counter-example
// via t.Errorf. A counter-example is a sample where the rule's premise
// (and its Given context, if present) holds but the conclusion does
// not — the rule claims an implication that the sample falsifies.
//
// Samples whose premise does not hold are silently skipped: the rule
// makes no claim about them. If every sample is a premise-miss, Verify
// does not fail — the rule is vacuously consistent with the sample
// set. Supply samples that genuinely cover the premise's support.
//
// Verify calls t.Helper() so failures point at the test function, not
// at this line.
func Verify[T any](t TestingT, rule infer.Rule, samples ...T) {
	t.Helper()
	for _, s := range samples {
		if !rule.Check(s) {
			t.Errorf("infertest.Verify: rule violated on sample %#v — premise holds but conclusion does not", s)
		}
	}
}

// VerifyApplies is the stricter variant: in addition to failing on
// counter-examples, it requires at least one sample to actually trigger
// the premise. If every sample is a premise-miss, the rule is vacuously
// "verified" — Verify would report success on a sample set that
// exercises nothing. VerifyApplies treats that as a test failure so
// the caller is forced to pick samples that cover the premise.
func VerifyApplies[T any](t TestingT, rule infer.Rule, samples ...T) {
	t.Helper()
	applied := 0
	for _, s := range samples {
		if rule.Applies(s) {
			applied++
		}
		if !rule.Check(s) {
			t.Errorf("infertest.VerifyApplies: rule violated on sample %#v — premise holds but conclusion does not", s)
		}
	}
	if applied == 0 {
		t.Errorf("infertest.VerifyApplies: no sample satisfied the rule's premise — %d samples supplied but the rule was vacuously true on all of them", len(samples))
	}
}
