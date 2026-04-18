// Caller guards with IsSmallPositive and calls target.T which
// requires IsPositive. The inference rule in the rules package
// provides IsSmallPositive ⇒ IsPositive. If rules' sidecar is
// written, the build succeeds; if not, the IsPositive obligation
// stays undischarged and the build fails.

package main

import (
	"fmt"

	"fixture/preds"
	"fixture/target"

	_ "fixture/rules"
)

func main() {
	x := 42
	if preds.IsSmallPositive(x) {
		fmt.Println(target.T(x))
	}
}
