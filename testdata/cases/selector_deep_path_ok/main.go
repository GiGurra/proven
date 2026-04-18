// Multi-segment selector paths canonicalize end-to-end: a guard on
// "request.Body.Amount" plants a fact keyed on exactly that path,
// and the downstream call taking the same path as its argument
// discharges without an intermediate local binding.

package main

import "github.com/GiGurra/proven/pkg/proven"

type Body struct {
	Amount int
}

type Request struct {
	Body Body
}

func isPositive(x int) bool { return x > 0 }

func accept(x int) {
	proven.That(x, isPositive)
}

func main() {
	req := Request{Body: Body{Amount: 5}}
	if isPositive(req.Body.Amount) {
		accept(req.Body.Amount)
	}
}
