// Generic predicates work as-is. `func nonNil[T any](p *T) bool`
// declared once is usable at every proven.That / prove.That site
// for any pointer type — Go's generic inference resolves T from the
// first argument's type, and the scanner resolves the predicate by
// its bare identifier before instantiation.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct {
	ID   int
	Name string
}

func nonNil[T any](p *T) bool { return p != nil }

func greet(u *User) {
	proven.That(u, nonNil)
	_ = u.Name // safe — greet promised nonNil
}

func main() {
	u := &User{ID: 1, Name: "Alice"}
	if nonNil(u) {
		greet(u)
	}
}
