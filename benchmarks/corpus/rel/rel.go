// Package rel exercises the tuple-subject pattern for "relations
// between values" documented in docs/relations.md.
package rel

import "github.com/GiGurra/proven/pkg/proven"

type Session struct{ ID string }
type User struct{ ID string }
type Resource struct{ ID string }

// AuthCtx packs the three subjects of the modify-permission relation.
// canModify is unary over the tuple; the relation is encoded by the
// struct shape.
type AuthCtx struct {
	S Session
	U User
	R Resource
}

func CanModify(a AuthCtx) bool {
	return a.S.ID != "" && a.U.ID != "" && a.R.ID != ""
}

func Modify(a AuthCtx) {
	proven.That(a, CanModify)
}

// Caller packs the tuple, guards via the unary predicate, and calls.
// Identical machinery to a primitive subject.
func Invoke(s Session, u User, r Resource) {
	a := AuthCtx{S: s, U: u, R: r}
	if CanModify(a) {
		Modify(a)
	}
}
