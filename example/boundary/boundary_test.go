package boundary

import (
	"errors"
	"testing"

	// Blank import of proventest supplies the _proven_atCompileTime symbol
	// so this test binary can link without the proven preprocessor.
	"github.com/GiGurra/proven/pkg/proventest"
	"github.com/GiGurra/proven/pkg/proven"
)

// 1. Boundary: valid inputs pass through prove.That and reach Transfer.
func TestHandleTransfer_ValidInputs(t *testing.T) {
	if err := HandleTransfer(100, "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 2. Boundary: invalid amount is rejected at prove.That with a
// proven.Violation error. Transfer is never called.
func TestHandleTransfer_RejectsNegativeAmount(t *testing.T) {
	err := HandleTransfer(-5, "hello")
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

// 3. Boundary: invalid note (empty) is rejected at prove.That.
func TestHandleTransfer_RejectsEmptyNote(t *testing.T) {
	err := HandleTransfer(100, "")
	if err == nil {
		t.Fatal("expected error")
	}
	var v proven.Violation
	if !errors.As(err, &v) {
		t.Fatalf("expected proven.Violation, got %T", err)
	}
}

// 4. Must variant panics on invalid input.
func TestHandleTransferMust_PanicsOnBadAmount(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if _, ok := r.(proven.Violation); !ok {
			t.Fatalf("expected proven.Violation, got %T: %v", r, r)
		}
	}()
	_ = HandleTransferMust(-5, "hello")
}

// 5. Wiring verification: Transfer's own proven.That is wired to
// isPositive on amount. If someone drops or weakens the assertion in
// Transfer, this test catches it.
func TestWiring_TransferAmountIsPositive(t *testing.T) {
	proventest.AssertFails(t, isPositive, func() {
		_ = Transfer(-5, "hello")
	})
}

// 6. Wiring verification: Transfer's note precondition includes isNonEmpty.
func TestWiring_TransferNoteIsNonEmpty(t *testing.T) {
	proventest.AssertFails(t, isNonEmpty, func() {
		_ = Transfer(10, "")
	})
}
