// Same generic nonNil predicate, but no guard at the call site — the
// build must fail with a cannot-prove diagnostic naming nonNil on the
// parameter that needed it.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct {
	ID   int
	Name string
}

func nonNil[T any](p *T) bool { return p != nil }

func greet(u *User) {
	proven.That(u, nonNil)
	_ = u.Name
}

func main() {
	var u *User // nil
	greet(u)    // no guard — cannot prove nonNil
}
