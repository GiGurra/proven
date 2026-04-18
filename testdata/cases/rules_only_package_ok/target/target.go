package target

import (
	"fixture/preds"

	"github.com/GiGurra/proven/pkg/proven"
)

func T(x int) int {
	proven.That(x, preds.IsPositive)
	return x
}
