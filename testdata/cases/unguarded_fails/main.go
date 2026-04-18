// A fixture where a caller invokes a precondition-annotated
// function without establishing the required predicate. Under
// -toolexec=proven the preprocessor emits an undischarged-
// obligation diagnostic and the build fails.

package main

import "github.com/GiGurra/proven/pkg/proven"

func isPositive(x int) bool { return x > 0 }

func target(amount int) {
	proven.That(amount, isPositive)
}

func main() {
	x := 5
	target(x)
}
