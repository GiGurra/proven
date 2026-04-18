// Passing the literal nil to a function that requires
// proven.NonNil fails the build.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct {
	ID int
}

func target(u *User) {
	proven.That(u, proven.NonNil)
}

func main() {
	target(nil)
}
