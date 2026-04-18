package proven

// A small library of ready-made predicates covering the most common
// shapes. Each is an ordinary func(T) bool that works as a predicate
// argument anywhere the preprocessor accepts one — proven.That,
// prove.That, trust.That, infer.From/Given/To, and guards.
//
// These predicates also unlock compile-time literal evaluation at
// call sites: when a caller passes a literal or simple constant
// expression to a function whose precondition is one of the names
// in this file, the preprocessor evaluates the predicate on the
// literal and accepts the call without a runtime check. The set of
// predicates the literal evaluator recognizes is keyed by the
// Predicate{Pkg, Name} identity — name, signature, and semantics
// must all stay in sync with internal/preprocessor/litconst.go.

// Numeric is the set of numeric kinds the ordering / zero
// predicates accept.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Integer is the set of integer kinds Even / Odd accept.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Positive reports whether v is strictly greater than zero.
func Positive[T Numeric](v T) bool { return v > 0 }

// Negative reports whether v is strictly less than zero.
func Negative[T Numeric](v T) bool { return v < 0 }

// NonNegative reports whether v is greater than or equal to zero.
func NonNegative[T Numeric](v T) bool { return v >= 0 }

// NonPositive reports whether v is less than or equal to zero.
func NonPositive[T Numeric](v T) bool { return v <= 0 }

// Zero reports whether v equals its type's zero value. Works on
// any comparable type: numerics (v == 0), strings (v == ""), bools
// (v == false), pointers / channels / interfaces / funcs carrying
// their nil form, arrays and structs whose fields are themselves
// comparable. Slice, map, and func-holding struct values that
// contain non-comparable fields are not comparable in Go, and
// calling Zero on them is a compile error — that's the same
// restriction the `comparable` constraint imposes on callers.
func Zero[T comparable](v T) bool {
	var zero T
	return v == zero
}

// NonZero reports whether v is not equal to its type's zero value.
// See Zero for the set of accepted types.
func NonZero[T comparable](v T) bool {
	var zero T
	return v != zero
}

// Even reports whether v is divisible by two.
func Even[T Integer](v T) bool { return v%2 == 0 }

// Odd reports whether v is not divisible by two.
func Odd[T Integer](v T) bool { return v%2 != 0 }

// NonEmpty reports whether s has at least one byte. Go's generics
// cannot express a single "supports len()" constraint across
// string, slice, map, and chan — slice / map types use a differently
// structured type-union that no constraint can enumerate — so the
// string variant keeps its simple signature and dedicated
// NonEmptySlice / NonEmptyMap predicates cover the parameterised
// container kinds.
func NonEmpty(s string) bool { return len(s) > 0 }

// Empty reports whether s is the empty string.
func Empty(s string) bool { return len(s) == 0 }

// NonEmptySlice reports whether s contains at least one element.
// A nil slice has length zero, so NonEmptySlice(nil) is false.
func NonEmptySlice[T any](s []T) bool { return len(s) > 0 }

// EmptySlice reports whether s contains no elements — including the
// nil slice, whose length is zero.
func EmptySlice[T any](s []T) bool { return len(s) == 0 }

// NonEmptyMap reports whether m has at least one key/value pair.
// A nil map has length zero, so NonEmptyMap(nil) is false.
func NonEmptyMap[K comparable, V any](m map[K]V) bool { return len(m) > 0 }

// EmptyMap reports whether m has no key/value pairs — including the
// nil map, whose length is zero.
func EmptyMap[K comparable, V any](m map[K]V) bool { return len(m) == 0 }

// NonNil reports whether p is not nil.
func NonNil[T any](p *T) bool { return p != nil }

// Nil reports whether p is nil.
func Nil[T any](p *T) bool { return p == nil }
