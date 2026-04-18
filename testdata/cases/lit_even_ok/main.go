// 4 is even at compile time.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Even)
}

func main() {
	target(4)
}
