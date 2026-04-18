// if x != nil plants proven.NonNil on x automatically — no explicit
// predicate call in the guard needed. The library predicate is
// keyed on the same identity so proven.That(u, proven.NonNil)
// downstream is satisfied from the comparison.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct {
	ID int
}

func target(u *User) {
	proven.That(u, proven.NonNil)
}

func maybeUser() *User { return &User{ID: 1} }

func main() {
	u := maybeUser()
	if u != nil {
		target(u)
	}
}
