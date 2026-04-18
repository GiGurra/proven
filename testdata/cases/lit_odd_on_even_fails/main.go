// 4 is not odd; the build fails.

package main

import "github.com/GiGurra/proven/pkg/proven"

func target(n int) {
	proven.That(n, proven.Odd)
}

func main() {
	target(4)
}
