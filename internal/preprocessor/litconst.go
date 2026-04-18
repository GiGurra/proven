package preprocessor

// Literal evaluation for library-known predicates.
//
// The preprocessor recognizes a small, fixed set of predicate names
// declared in pkg/proven/predicates.go — Positive, NonNil, NonEmpty
// etc. — and can evaluate them at build time on literal argument
// expressions. When the call site passes a literal whose value
// satisfies the required predicate, the precondition is accepted
// without a runtime check.
//
// Scope is deliberately narrow:
//
//   - Only predicates with Predicate{Pkg: provenImportPath, Name: X}
//     where X is one of the names in pkg/proven/predicates.go are
//     evaluated. User-defined predicates — even those that are
//     semantically identical — are not.
//   - Only simple argument shapes are recognized: BasicLit (INT /
//     FLOAT / STRING), a UnaryExpr applying `-` to a BasicLit
//     (negative numeric literals), the bare identifier `nil`,
//     address-of a CompositeLit (`&T{…}` — never nil), `new(T)` and
//     `make(...)` (also never nil). Package-level Go `const`
//     references are NOT yet resolved (follow-up work).
//
// Anything outside this scope is reported as EvalSkip and the caller
// falls through to the regular discharge paths — guards, advertised
// postconditions, rules, trust.That, or the cannot-prove diagnostic.

import (
	"go/ast"
	"go/token"
	"strconv"
)

// EvalResult is the outcome of evaluating a library predicate on a
// literal argument expression.
type EvalResult int

const (
	// EvalSkip: the predicate is not library-known, the argument
	// expression is not in the evaluator's recognized shape set, or
	// the combination is not applicable. The caller falls back to
	// normal discharge.
	EvalSkip EvalResult = iota
	// EvalHolds: the predicate evaluates to true on the literal.
	EvalHolds
	// EvalFails: the predicate evaluates to false on the literal.
	// The caller treats this as "cannot prove" (the literal cannot
	// satisfy the predicate on any call).
	EvalFails
)

// evalLibraryPredicate attempts to evaluate p on arg at build time.
// Returns EvalSkip when the predicate or argument shape is outside
// the evaluator's scope — the caller should continue through the
// normal discharge paths.
func evalLibraryPredicate(p Predicate, arg ast.Expr) EvalResult {
	if p.Pkg != provenImportPath {
		return EvalSkip
	}
	switch p.Name {
	case "NonNil":
		return evalPointerIsNil(arg, false)
	case "Nil":
		return evalPointerIsNil(arg, true)
	case "NonEmpty":
		return evalStringLen(arg, func(n int) bool { return n > 0 })
	case "Empty":
		return evalStringLen(arg, func(n int) bool { return n == 0 })
	case "Positive":
		return evalNumeric(arg, cmpPositive)
	case "Negative":
		return evalNumeric(arg, cmpNegative)
	case "NonNegative":
		return evalNumeric(arg, cmpNonNegative)
	case "NonPositive":
		return evalNumeric(arg, cmpNonPositive)
	case "Zero":
		return evalNumeric(arg, cmpZero)
	case "NonZero":
		return evalNumeric(arg, cmpNonZero)
	case "Even":
		return evalInteger(arg, cmpEven)
	case "Odd":
		return evalInteger(arg, cmpOdd)
	}
	return EvalSkip
}

// --- numeric predicate cores ---------------------------------------

type intCmp func(int64) bool
type floatCmp func(float64) bool

func cmpPositive() (intCmp, floatCmp) {
	return func(n int64) bool { return n > 0 }, func(f float64) bool { return f > 0 }
}
func cmpNegative() (intCmp, floatCmp) {
	return func(n int64) bool { return n < 0 }, func(f float64) bool { return f < 0 }
}
func cmpNonNegative() (intCmp, floatCmp) {
	return func(n int64) bool { return n >= 0 }, func(f float64) bool { return f >= 0 }
}
func cmpNonPositive() (intCmp, floatCmp) {
	return func(n int64) bool { return n <= 0 }, func(f float64) bool { return f <= 0 }
}
func cmpZero() (intCmp, floatCmp) {
	return func(n int64) bool { return n == 0 }, func(f float64) bool { return f == 0 }
}
func cmpNonZero() (intCmp, floatCmp) {
	return func(n int64) bool { return n != 0 }, func(f float64) bool { return f != 0 }
}

// evalNumeric parses a numeric literal (int / float / signed int /
// signed float) and applies the predicate returned by cmp. Unsigned
// literals fall into the int branch and negate-then-fail when the
// source text has a leading minus, which is Go-correct for ~int
// kinds but the evaluator does not know the target type — a
// negative literal paired with a uint parameter would fail to
// compile anyway, so the evaluator does not try to second-guess
// Go's type checker.
func evalNumeric(arg ast.Expr, cmp func() (intCmp, floatCmp)) EvalResult {
	iCmp, fCmp := cmp()
	if i, ok := asIntLit(arg); ok {
		if iCmp(i) {
			return EvalHolds
		}
		return EvalFails
	}
	if f, ok := asFloatLit(arg); ok {
		if fCmp(f) {
			return EvalHolds
		}
		return EvalFails
	}
	return EvalSkip
}

type intCmp1 func(int64) bool

func cmpEven() intCmp1 { return func(n int64) bool { return n%2 == 0 } }
func cmpOdd() intCmp1  { return func(n int64) bool { return n%2 != 0 } }

// evalInteger is the integer-only variant for Even / Odd. Floats
// are reported as EvalSkip — the predicate is not meaningful and
// Go's type system would reject the call anyway.
func evalInteger(arg ast.Expr, cmp func() intCmp1) EvalResult {
	iCmp := cmp()
	if i, ok := asIntLit(arg); ok {
		if iCmp(i) {
			return EvalHolds
		}
		return EvalFails
	}
	return EvalSkip
}

// asIntLit accepts INT BasicLit and UnaryExpr{SUB, INT BasicLit}.
func asIntLit(arg ast.Expr) (int64, bool) {
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.SUB {
		if v, ok := asIntLit(u.X); ok {
			return -v, true
		}
		return 0, false
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	// Go integer literal: accept decimal, hex (0x), octal (0 / 0o),
	// binary (0b). strconv.ParseInt with base 0 handles all four.
	n, err := strconv.ParseInt(lit.Value, 0, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// asFloatLit accepts FLOAT BasicLit and UnaryExpr{SUB, FLOAT BasicLit}.
// Also accepts INT BasicLit promoted to float for predicates that
// care about ordering only (Positive, Negative, etc. — Zero and
// NonZero also coincide with the integer case on non-zero ints). We
// keep the two asFooLit helpers separate because the evaluator
// dispatches on whichever matches first, so INT literals go through
// asIntLit and only genuine FLOAT literals land here.
func asFloatLit(arg ast.Expr) (float64, bool) {
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.SUB {
		if v, ok := asFloatLit(u.X); ok {
			return -v, true
		}
		return 0, false
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.FLOAT {
		return 0, false
	}
	f, err := strconv.ParseFloat(lit.Value, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// --- string -------------------------------------------------------

// evalStringLen applies cmp to the UNQUOTED length of a string
// literal. Raw strings (backtick-quoted) and regular quoted strings
// both work — strconv.Unquote handles both.
func evalStringLen(arg ast.Expr, cmp func(int) bool) EvalResult {
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return EvalSkip
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return EvalSkip
	}
	if cmp(len(s)) {
		return EvalHolds
	}
	return EvalFails
}

// --- pointer nil / non-nil ----------------------------------------

// evalPointerIsNil returns EvalHolds when arg is known at compile
// time to have the requested nil-ness (wantNil == true for Nil,
// false for NonNil). Recognized shapes:
//
//   - `nil` identifier (known nil)
//   - `&X{...}` address-of a composite literal (known non-nil)
//   - `new(T)` (known non-nil)
//   - `make(...)` (known non-nil when T is a map / slice / chan /
//     pointer; make produces non-nil of those kinds unconditionally)
//
// Anything else is EvalSkip.
func evalPointerIsNil(arg ast.Expr, wantNil bool) EvalResult {
	if id, ok := arg.(*ast.Ident); ok && id.Name == "nil" {
		if wantNil {
			return EvalHolds
		}
		return EvalFails
	}
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.AND {
		if _, ok := u.X.(*ast.CompositeLit); ok {
			if wantNil {
				return EvalFails
			}
			return EvalHolds
		}
	}
	if c, ok := arg.(*ast.CallExpr); ok {
		if id, ok := c.Fun.(*ast.Ident); ok {
			switch id.Name {
			case "new", "make":
				if wantNil {
					return EvalFails
				}
				return EvalHolds
			}
		}
	}
	return EvalSkip
}
