// A relation between multiple values, expressed as a unary predicate
// over a domain struct that packs the participating subjects. The
// existing unary analyzer handles this without any preprocessor
// changes — see docs/relations.md for why tuple-subject is the v1
// approach to relations.

package main

import "github.com/GiGurra/proven/pkg/proven"

type Session struct{ ID string }
type User struct{ ID string }
type Resource struct{ ID string }

// AuthCtx packs the three subjects of the modify-permission relation.
type AuthCtx struct {
	S Session
	U User
	R Resource
}

// canModify is unary over AuthCtx; the "relation" is encoded by the
// struct shape.
func canModify(a AuthCtx) bool {
	return a.S.ID != "" && a.U.ID != "" && a.R.ID != ""
}

func modifyResource(a AuthCtx) {
	proven.That(a, canModify)
}

func main() {
	a := AuthCtx{S: Session{ID: "s1"}, U: User{ID: "u1"}, R: Resource{ID: "r1"}}
	if canModify(a) {
		modifyResource(a)
	}
}
