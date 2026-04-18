// Taking the address of x and passing it to a function lets that
// function mutate *x invisibly to the analyzer. Conservatively
// forget x's facts once &x has escaped.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Positive)
}

func mutate(p *int) { *p = -1 }

func main() {
	x := 42
	if proven.Positive(x) {
		mutate(&x)   // &x escapes — any future value of x is opaque
		target(x) // cannot prove — x may have been mutated
	}
}
