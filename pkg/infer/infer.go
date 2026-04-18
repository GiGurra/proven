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
// Each slot (From, Given, To) is variadic and AND-composes its
// arguments — the same semantics as proven.That / prove.That /
// trust.That. A multi-argument From means "every premise must hold";
// a multi-argument To means "every conclusion follows". The single-
// argument form above is just the trivial case:
//
//	var _ = infer.From(isEven, isPositive).To(isNonNeg, isNonZero)
//	// reads: if isEven(x) AND isPositive(x), we can conclude
//	// isNonNeg(x) AND isNonZero(x).
//
// The builder reads left-to-right as the logical statement: "from
// this premise [, given this context], we conclude to". The terminal
// .To call returns a Rule value, which exists only so declarations
// can sit at package scope via `var _ = ...`.
//
// # Trust model
//
// Rules are trusted — the preprocessor does not attempt to symbolically
// verify that the declared implication actually holds. That would
// require full SMT, which is out of scope. Declarers are responsible
// for the soundness of their rules. See infertest for property-style
// runtime checking.
package infer

// Rule is the declared inference fact. The preprocessor reads these
// declarations from the AST and has no need of the runtime value, but
// the predicates are retained so infertest (or any other runtime-
// verification helper) can evaluate the implication against sample
// inputs. Rule is intentionally non-generic so declarations can sit
// uniformly at package scope; the typed predicates are held behind
// `func(any) bool` wrappers that type-assert on entry.
//
// from / given / to are slices (AND-composed within each slot) to
// mirror the variadic builder surface. given is nil when no .Given
// step was used.
type Rule struct {
	from  []func(any) bool
	given []func(any) bool
	to    []func(any) bool
}

// Check evaluates the rule on one sample value. Returns true when the
// rule is either vacuously satisfied (some premise — or some Given,
// if present — does not hold) or genuinely satisfied (every premise
// and every Given hold, and every conclusion also holds). Returns
// false only when every premise (and every Given) hold but some
// conclusion fails — a counter-example.
//
// A sample whose dynamic type does not match the rule's predicate
// type reports false from the wrapper's type assertion, producing a
// silent premise-miss (return true). This keeps Check from panicking
// on mismatched types; verifiers should call it with samples of the
// rule's declared type.
func (r Rule) Check(sample any) bool {
	if len(r.from) == 0 {
		return true
	}
	if !allHold(r.from, sample) {
		return true
	}
	if len(r.given) > 0 && !allHold(r.given, sample) {
		return true
	}
	return allHold(r.to, sample)
}

// Applies reports whether every premise (and every Given, if present)
// holds on sample — i.e. whether Check's result on this sample is
// load-bearing. Useful for verifiers that want to distinguish
// "rule did not apply" from "rule applied and was satisfied".
func (r Rule) Applies(sample any) bool {
	if len(r.from) == 0 {
		return false
	}
	if !allHold(r.from, sample) {
		return false
	}
	if len(r.given) > 0 && !allHold(r.given, sample) {
		return false
	}
	return true
}

func allHold(preds []func(any) bool, sample any) bool {
	for _, p := range preds {
		if !p(sample) {
			return false
		}
	}
	return true
}

// From begins building an inference rule with one or more premise
// predicates, AND-composed. Chain .Given (optional) and .To to
// complete it.
func From[T any](premises ...func(T) bool) FromStep[T] {
	return FromStep[T]{from: premises}
}

// FromStep is the intermediate builder after From.
type FromStep[T any] struct {
	from []func(T) bool
}

// Given narrows the rule to values that also satisfy every context
// predicate. Multiple arguments AND-compose.
func (f FromStep[T]) Given(contexts ...func(T) bool) GivenStep[T] {
	return GivenStep[T]{from: f.from, given: contexts}
}

// To completes an unconditional rule: from ⇒ conclusion. Multiple
// conclusion arguments AND-compose — the rule asserts every one.
func (f FromStep[T]) To(conclusions ...func(T) bool) Rule {
	return Rule{
		from: wrapAll(f.from),
		to:   wrapAll(conclusions),
	}
}

// GivenStep is the intermediate builder after Given.
type GivenStep[T any] struct {
	from, given []func(T) bool
}

// To completes a conditional rule: from ∧ given ⇒ conclusion.
// Multiple conclusion arguments AND-compose.
func (g GivenStep[T]) To(conclusions ...func(T) bool) Rule {
	return Rule{
		from:  wrapAll(g.from),
		given: wrapAll(g.given),
		to:    wrapAll(conclusions),
	}
}

// wrapAll lifts a slice of typed predicates into the `func(any) bool`
// form the non-generic Rule carries.
func wrapAll[T any](preds []func(T) bool) []func(any) bool {
	if len(preds) == 0 {
		return nil
	}
	out := make([]func(any) bool, len(preds))
	for i, p := range preds {
		out[i] = wrapAny(p)
	}
	return out
}

// wrapAny lifts a single typed predicate. A dynamic-type mismatch
// reports false so Check degrades to a premise-miss rather than
// panicking.
func wrapAny[T any](p func(T) bool) func(any) bool {
	return func(v any) bool {
		t, ok := v.(T)
		if !ok {
			return false
		}
		return p(t)
	}
}
