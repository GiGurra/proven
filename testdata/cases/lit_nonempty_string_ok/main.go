// A non-empty string literal satisfies proven.NonEmpty at build time.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(s string) {
	proven.That(s, proven.NonEmpty)
}

func main() {
	target("hello")
}
