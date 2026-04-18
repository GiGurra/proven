// proven.Or inside an infer slot is rejected — a disjunctive premise
// or conclusion would synthesize multiple rules; the user is asked
// to declare them explicitly.

package main

import (
	"github.com/GiGurra/proven/pkg/infer"
	"github.com/GiGurra/proven/pkg/proven"
)

func isPositive(x int) bool  { return x > 0 }
func lessThan100(x int) bool { return x < 100 }
func isNonNeg(x int) bool    { return x >= 0 }

var _ = infer.From(proven.Or(isPositive, lessThan100)).To(isNonNeg)

func main() {}
