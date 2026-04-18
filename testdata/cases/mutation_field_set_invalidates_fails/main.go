// A field write on a tuple subject invalidates the relation
// predicate: after `ctx.U = nil`, the `canModify` check that
// passed on the original ctx cannot still be trusted.

package main

import "github.com/GiGurra/proven/pkg/proven"

type User struct{ ID int }

type AuthCtx struct {
	U *User
	OK bool
}

func canModify(a AuthCtx) bool { return a.U != nil && a.OK }

func modifyResource(a AuthCtx) {
	proven.That(a, canModify)
}

func main() {
	a := AuthCtx{U: &User{ID: 1}, OK: true}
	if canModify(a) {
		a.U = nil              // mutation of a's field invalidates facts on a
		modifyResource(a) // cannot prove — canModify no longer guaranteed
	}
}
