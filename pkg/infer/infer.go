// Package infer declares inference rules that the proven preprocessor
// uses to discharge obligations by implication.
//
// When a caller has established predicate P on a value and a callee
// requires predicate Q, the preprocessor can automatically discharge
// the obligation if an inference rule declares P ⇒ Q (optionally under
// a context condition). Rules live at package scope as tiny fluent
// builder invocations:
//
//	var _ = infer.From(isSmallPositive).To(isPositive)
//	var _ = infer.From(isEven).Given(isGreaterThanZero).To(isPositive)
//
// The builder reads left-to-right as the logical statement: "from this
// premise [, given this context], we conclude to". The terminal .To
// call returns a Rule value, which exists only so declarations can sit
// at package scope via `var _ = ...`.
//
// # Trust model
//
// Rules are trusted — the preprocessor does not attempt to symbolically
// verify that the declared implication actually holds. That would
// require full SMT, which is out of scope. Declarers are responsible
// for the soundness of their rules. See infertest (future) for
// property-style runtime checking.
//
// # Companion concepts
//
// A future addition to this package is compile-time evaluation of pure
// expressions (comptime), e.g. infer.Const(sieve(10_000)). Both
// capabilities fit the "compile-time deduction" theme: one infers
// values, the other infers relationships between predicates.
package infer

// Rule is the declared inference fact. The preprocessor reads these
// declarations from the AST and has no need of the runtime value, but
// the predicates are retained so infertest (or any other runtime-
// verification helper) can evaluate the implication against sample
// inputs. Rule is intentionally non-generic so declarations can sit
// uniformly at package scope; the typed predicates are held behind
// `func(any) bool` wrappers that type-assert on entry.
type Rule struct {
	from  func(any) bool
	given func(any) bool // nil when no .Given step was used
	to    func(any) bool
}

// Check evaluates the rule on one sample value. Returns true when the
// rule is either vacuously satisfied (premise — or Given, if present —
// does not hold) or genuinely satisfied (premise and Given both hold
// and conclusion also holds). Returns false only when the premise
// (and Given) hold but the conclusion fails — a counter-example.
//
// A sample whose dynamic type does not match the rule's predicate
// type reports false from the wrapper's type assertion, producing a
// silent premise-miss (return true). This keeps Check from panicking
// on mismatched types; verifiers should call it with samples of the
// rule's declared type.
func (r Rule) Check(sample any) bool {
	if r.from == nil {
		return true
	}
	if !r.from(sample) {
		return true
	}
	if r.given != nil && !r.given(sample) {
		return true
	}
	return r.to(sample)
}

// Applies reports whether the rule's premise (and Given, if present)
// hold on sample — i.e. whether Check's result on this sample is
// load-bearing. Useful for verifiers that want to distinguish
// "rule did not apply" from "rule applied and was satisfied".
func (r Rule) Applies(sample any) bool {
	if r.from == nil {
		return false
	}
	if !r.from(sample) {
		return false
	}
	if r.given != nil && !r.given(sample) {
		return false
	}
	return true
}

// From begins building an inference rule with a premise predicate.
// Chain .Given (optional) and .To to complete it.
func From[T any](premise func(T) bool) FromStep[T] {
	return FromStep[T]{from: premise}
}

// FromStep is the intermediate builder after From.
type FromStep[T any] struct {
	from func(T) bool
}

// Given narrows the rule to values that also satisfy context. For
// multiple context predicates, compose with proven.And.
func (f FromStep[T]) Given(context func(T) bool) GivenStep[T] {
	return GivenStep[T]{from: f.from, given: context}
}

// To completes an unconditional rule: from ⇒ conclusion.
func (f FromStep[T]) To(conclusion func(T) bool) Rule {
	return Rule{
		from: wrapAny(f.from),
		to:   wrapAny(conclusion),
	}
}

// GivenStep is the intermediate builder after Given.
type GivenStep[T any] struct {
	from, given func(T) bool
}

// To completes a conditional rule: from ∧ given ⇒ conclusion.
func (g GivenStep[T]) To(conclusion func(T) bool) Rule {
	return Rule{
		from:  wrapAny(g.from),
		given: wrapAny(g.given),
		to:    wrapAny(conclusion),
	}
}

// wrapAny lifts a typed predicate into the `func(any) bool` form the
// non-generic Rule carries. A dynamic-type mismatch reports false so
// Check degrades to a premise-miss rather than panicking.
func wrapAny[T any](p func(T) bool) func(any) bool {
	return func(v any) bool {
		t, ok := v.(T)
		if !ok {
			return false
		}
		return p(t)
	}
}
