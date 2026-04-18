package m02

import (
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l00"
	"github.com/GiGurra/proven/benchmarks/corpus/leaf/l02"
)

// Discharge via proven.Returns postcondition: Clamp returns a value
// with IsNonNeg; Target requires IsPositive — the rules package
// doesn't have IsNonNeg ⇒ IsPositive (that's false), so we use
// MakePositive which returns IsPositive directly.
func Normalize(x int) int {
	v := l02.MakePositive(x)
	return l00.Target(v) // IsPositive fact flows through
}

func Double(x int) int {
	p := l02.MakePositive(x)
	return l00.Target(p) * 2
}
