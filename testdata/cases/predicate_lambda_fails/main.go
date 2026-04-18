// A function literal as the predicate argument to proven.That has no
// stable package+name identity, so the preprocessor cannot correlate
// it with discharge attempts at any call site. Under strict mode the
// scanner refuses to silently drop the obligation and fails the build
// with a diagnostic pointing at the lambda.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(x int) {
	proven.That(x, func(n int) bool { return n > 0 })
}

func main() {
	target(5)
}
