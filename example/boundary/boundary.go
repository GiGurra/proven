// Package boundary demonstrates the prove -> proven flow: external
// data is validated once at the program boundary with prove.That, and
// downstream functions with proven.That preconditions are called with
// the validated values — the preprocessor recognises the prove.That
// output as satisfying the downstream preconditions without any
// re-validation.
package boundary

import (
	"github.com/GiGurra/proven/pkg/prove"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool    { return x > 0 }
func isNonEmpty(s string) bool { return len(s) > 0 }
func maxLen280(s string) bool  { return len(s) <= 280 }

// Transfer is the internal domain function. It declares preconditions
// on its inputs. Under the preprocessor these are obligations every
// caller must discharge; outside a proven.That block, they are the
// material the preprocessor reads.
func Transfer(amount int, note string) error {
	proven.That(amount, isPositive)
	proven.That(note, isNonEmpty, maxLen280)
	_ = amount
	_ = note
	return nil
}

// HandleTransfer is the boundary function — it accepts raw external
// input and validates it once with prove.That. Each successful
// prove.That establishes a flow-sensitive fact the preprocessor uses
// to discharge Transfer's proven.That obligations automatically.
func HandleTransfer(rawAmount int, rawNote string) error {
	amount, err := prove.That(rawAmount, isPositive)
	if err != nil {
		return err
	}
	note, err := prove.That(rawNote, isNonEmpty, maxLen280)
	if err != nil {
		return err
	}
	return Transfer(amount, note)
}

// HandleTransferMust is the same boundary, but with prove.Must — used
// in startup-style code where validation failure is fatal and a panic
// is the correct response.
func HandleTransferMust(rawAmount int, rawNote string) error {
	amount := prove.Must(rawAmount, isPositive)
	note := prove.Must(rawNote, isNonEmpty, maxLen280)
	return Transfer(amount, note)
}
