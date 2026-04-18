// Early-return nil guard: after `if x == nil { return }` the
// analyzer knows x is non-nil for the rest of the function, so a
// downstream proven.NonNil precondition is satisfied with no
// explicit predicate call.

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
	if u == nil {
		return
	}
	target(u) // post-guard: u is known non-nil
}
