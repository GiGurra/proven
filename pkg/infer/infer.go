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

// Rule is the declared inference fact. It has no runtime behavior;
// the value exists only so declarations can live at package scope.
type Rule struct{}

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
	_ = f.from
	_ = conclusion
	return Rule{}
}

// GivenStep is the intermediate builder after Given.
type GivenStep[T any] struct {
	from, given func(T) bool
}

// To completes a conditional rule: from ∧ given ⇒ conclusion.
func (g GivenStep[T]) To(conclusion func(T) bool) Rule {
	_ = g.from
	_ = g.given
	_ = conclusion
	return Rule{}
}
