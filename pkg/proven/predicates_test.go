package proven

import "testing"

// TestZero_Numeric covers the previous Numeric-only behaviour: the
// generalised comparable predicate still agrees with the intuitive
// numeric zero / non-zero answer.
func TestZero_Numeric(t *testing.T) {
	if !Zero(0) {
		t.Error("Zero(0) = false, want true")
	}
	if Zero(1) {
		t.Error("Zero(1) = true, want false")
	}
	if !Zero(0.0) {
		t.Error("Zero(0.0) = false, want true")
	}
	if Zero(-0.5) {
		t.Error("Zero(-0.5) = true, want false")
	}
	if NonZero(0) {
		t.Error("NonZero(0) = true, want false")
	}
	if !NonZero(7) {
		t.Error("NonZero(7) = false, want true")
	}
}

// TestZero_String locks the new widened behaviour: an empty string
// is the zero value of string; a non-empty string is not.
func TestZero_String(t *testing.T) {
	if !Zero("") {
		t.Error(`Zero("") = false, want true`)
	}
	if Zero("hi") {
		t.Error(`Zero("hi") = true, want false`)
	}
	if NonZero("") {
		t.Error(`NonZero("") = true, want false`)
	}
	if !NonZero("hi") {
		t.Error(`NonZero("hi") = false, want true`)
	}
}

// TestZero_Bool covers the bool-zero case: false is Go's zero
// value for bool, true is not.
func TestZero_Bool(t *testing.T) {
	if !Zero(false) {
		t.Error("Zero(false) = false, want true")
	}
	if Zero(true) {
		t.Error("Zero(true) = true, want false")
	}
}

// TestZero_Pointer covers pointer zero (nil) vs non-nil pointer.
func TestZero_Pointer(t *testing.T) {
	var nilPtr *int
	x := 7
	if !Zero(nilPtr) {
		t.Error("Zero(nil *int) = false, want true")
	}
	if Zero(&x) {
		t.Error("Zero(&x) = true, want false")
	}
	if NonZero(nilPtr) {
		t.Error("NonZero(nil *int) = true, want false")
	}
	if !NonZero(&x) {
		t.Error("NonZero(&x) = false, want true")
	}
}

// TestZero_Struct covers a comparable struct: the zero struct vs a
// struct with a non-zero field.
func TestZero_Struct(t *testing.T) {
	type point struct {
		X, Y int
	}
	if !Zero(point{}) {
		t.Error("Zero(point{}) = false, want true")
	}
	if Zero(point{X: 1}) {
		t.Error("Zero(point{X: 1}) = true, want false")
	}
	if NonZero(point{}) {
		t.Error("NonZero(point{}) = true, want false")
	}
	if !NonZero(point{Y: 2}) {
		t.Error("NonZero(point{Y: 2}) = false, want true")
	}
}

// TestNonEmptySlice covers the new parameterised slice predicate:
// nil and empty slices are "empty"; non-empty slices are not.
func TestNonEmptySlice(t *testing.T) {
	if NonEmptySlice([]int{}) {
		t.Error("NonEmptySlice(empty) = true, want false")
	}
	if NonEmptySlice[int](nil) {
		t.Error("NonEmptySlice(nil) = true, want false")
	}
	if !NonEmptySlice([]int{1}) {
		t.Error("NonEmptySlice([1]) = false, want true")
	}
	if !EmptySlice([]int{}) {
		t.Error("EmptySlice(empty) = false, want true")
	}
	if !EmptySlice[int](nil) {
		t.Error("EmptySlice(nil) = false, want true")
	}
	if EmptySlice([]int{1}) {
		t.Error("EmptySlice([1]) = true, want false")
	}
}

// TestNonEmptySlice_AnyType covers non-int element types to pin
// that the predicate is truly generic.
func TestNonEmptySlice_AnyType(t *testing.T) {
	if !NonEmptySlice([]string{"a", "b"}) {
		t.Error(`NonEmptySlice(["a","b"]) = false, want true`)
	}
	if EmptySlice([]string{"a"}) {
		t.Error(`EmptySlice(["a"]) = true, want false`)
	}
}

// TestNonEmptyMap covers the parameterised map predicate on typical
// and nil shapes.
func TestNonEmptyMap(t *testing.T) {
	if NonEmptyMap(map[string]int{}) {
		t.Error("NonEmptyMap(empty) = true, want false")
	}
	if NonEmptyMap[string, int](nil) {
		t.Error("NonEmptyMap(nil) = true, want false")
	}
	if !NonEmptyMap(map[string]int{"a": 1}) {
		t.Error(`NonEmptyMap({"a":1}) = false, want true`)
	}
	if !EmptyMap(map[string]int{}) {
		t.Error("EmptyMap(empty) = false, want true")
	}
	if !EmptyMap[string, int](nil) {
		t.Error("EmptyMap(nil) = false, want true")
	}
	if EmptyMap(map[string]int{"a": 1}) {
		t.Error(`EmptyMap({"a":1}) = true, want false`)
	}
}

// TestNonEmpty_StringStillWorks covers the pre-existing string
// predicates to ensure the new Slice / Map additions did not disturb
// the simpler string-only names users already rely on.
func TestNonEmpty_StringStillWorks(t *testing.T) {
	if !NonEmpty("hi") {
		t.Error(`NonEmpty("hi") = false, want true`)
	}
	if NonEmpty("") {
		t.Error(`NonEmpty("") = true, want false`)
	}
	if !Empty("") {
		t.Error(`Empty("") = false, want true`)
	}
	if Empty("hi") {
		t.Error(`Empty("hi") = true, want false`)
	}
}
