// An empty string literal fails proven.NonEmpty; the build fails.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(s string) {
	proven.That(s, proven.NonEmpty)
}

func main() {
	target("")
}
