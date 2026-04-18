// Mirror of tuple_relation_guard_discharges_ok, but without the
// guard: the caller never checks canModify(a) before modifyResource(a).
// The preprocessor must flag the obligation as undischarged — the
// same diagnostic shape as any other unary undischarged predicate,
// because the tuple relation rides on the unary machinery.

package main

import "github.com/GiGurra/proven/pkg/proven"

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
	a := AuthCtx{S: Session{ID: "s1"}, U: User{ID: "u1"}, R: Resource{ID: "r1"}}
	modifyResource(a) // no guard — should fail the build
}
