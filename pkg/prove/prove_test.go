package prove_test

import (
	"errors"
	"testing"

	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool    { return x > 0 }
func lessThan100(x int) bool   { return x < 100 }
func isNonEmpty(s string) bool { return len(s) > 0 }

func TestThat_HappyPath(t *testing.T) {
	got, err := prove.That(42, isPositive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

func TestThat_MultiplePredicates_AllPass(t *testing.T) {
	got, err := prove.That(50, isPositive, lessThan100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 50 {
		t.Fatalf("want 50, got %d", got)
	}
}

func TestThat_FirstPredicateFails(t *testing.T) {
	_, err := prove.That(-5, isPositive)
	if err == nil {
		t.Fatal("expected error")
	}
	var v proven.Violation
	if !errors.As(err, &v) {
		t.Fatalf("expected proven.Violation, got %T: %v", err, err)
	}
	if v.Value != -5 {
		t.Errorf("want Value=-5, got %v", v.Value)
	}
}

func TestThat_SecondPredicateFails(t *testing.T) {
	_, err := prove.That(200, isPositive, lessThan100)
	if err == nil {
		t.Fatal("expected error on second predicate")
	}
	var v proven.Violation
	if !errors.As(err, &v) {
		t.Fatalf("expected proven.Violation, got %T", err)
	}
	if proven.PredicateName(v.Predicate) != proven.PredicateName(lessThan100) {
		t.Errorf("want lessThan100 to fire, got %s", proven.PredicateName(v.Predicate))
	}
}

func TestThat_StringPredicate(t *testing.T) {
	got, err := prove.That("hello", isNonEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("want 'hello', got %q", got)
	}
}

func TestMust_HappyPath(t *testing.T) {
	got := prove.Must(42, isPositive)
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

func TestMust_PanicsOnViolation(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		v, ok := r.(proven.Violation)
		if !ok {
			t.Fatalf("expected proven.Violation, got %T: %v", r, r)
		}
		if v.Value != -1 {
			t.Errorf("want Value=-1, got %v", v.Value)
		}
	}()
	_ = prove.Must(-1, isPositive)
}
