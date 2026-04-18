// &T{} is known-non-nil at compile time; proven.NonNil is
// satisfied without a runtime check.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct {
	ID   int
	Name string
}

func target(u *User) {
	proven.That(u, proven.NonNil)
}

func main() {
	target(&User{ID: 1, Name: "Alice"})
}
