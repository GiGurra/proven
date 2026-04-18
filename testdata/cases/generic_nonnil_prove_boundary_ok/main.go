// prove.That at a nil-producing boundary: the runtime check returns
// an error on nil, and the err==nil branch plants nonNil as a fact
// on the returned pointer. Downstream callees requiring nonNil are
// satisfied without a re-check.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/prove"
)

type User struct {
	ID   int
	Name string
}

func nonNil[T any](p *T) bool { return p != nil }

func greet(u *User) {
	proven.That(u, nonNil)
	_ = u.Name
}

func lookupUser(id int) *User {
	if id == 1 {
		return &User{ID: 1, Name: "Alice"}
	}
	return nil
}

func main() {
	u, err := prove.That(lookupUser(1), nonNil)
	if err != nil {
		return
	}
	greet(u)
}
