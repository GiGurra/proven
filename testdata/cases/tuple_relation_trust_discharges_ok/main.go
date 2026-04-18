// trust.That composes with tuple-subject relations: asserting a
// relational predicate via trust.That on a tuple value injects the
// fact on the LHS, discharging a downstream proven.That that requires
// the same predicate on the same tuple. Verifies that nothing in the
// existing analyzer machinery is tuple-hostile.

package main

import (
	"github.com/GiGurra/proven/pkg/proven"
	"github.com/GiGurra/proven/pkg/trust"
)

type Session struct{ ID string }
type User struct{ ID string }
type Resource struct{ ID string }

type AuthCtx struct {
	S Session
	U User
	R Resource
}

func canModify(a AuthCtx) bool {
	return a.S.ID != "" && a.U.ID != "" && a.R.ID != ""
}

func modifyResource(a AuthCtx) {
	proven.That(a, canModify)
}

func main() {
	raw := AuthCtx{S: Session{ID: "s1"}, U: User{ID: "u1"}, R: Resource{ID: "r1"}}
	// Some external mechanism (schema validator, upstream audit, etc.)
	// has already established canModify(raw); we decline to repeat
	// the check and trust.That carries the fact forward.
	a := trust.That(raw, canModify)
	modifyResource(a)
}
