// Package trust is the "trust me" escape hatch in proven: the one
// place where you assert a compile-time fact on your own word
// instead of having the compiler or a runtime check stand behind
// it. The preprocessor accepts the fact, erases the call, and
// carries it forward like any other discharge path. If your
// assertion is wrong, nothing catches it at build or runtime;
// only the downstream code relying on the fact misbehaves.
//
// It is the third shape on the verification-cost / verification-
// strength axis the proven system exposes:
//
//	prove.That(v, preds...) (T, error)  — runtime check, error return
//	prove.Must(v, preds...) T           — runtime check, panic on fail
//	trust.That(v, preds...) T           — no check, asserts the fact
//
// Reach for trust.That when the value is known to satisfy the
// predicate through some mechanism the preprocessor cannot see:
//
//   - an external schema validator (JSON schema, protobuf
//     validate, database CHECK constraint) has already rejected
//     bad inputs upstream;
//   - business logic you've audited establishes the invariant on
//     this path but in a way the analyzer would need
//     interprocedural or type-state reasoning to see;
//   - a generated decoder already performs equivalent validation
//     and duplicating the check here buys no safety.
//
// The contract is: you assert the predicates, proven's analyzer
// treats the result as a fact downstream. If your assertion is
// wrong, there is no runtime defense — only the bug manifesting
// later in whichever code consumed the proven fact. Use
// prove.Must when you want a defensive runtime check; use
// trust.That when you have weighed the cost and are confident.
//
// Distinct from proven.Returns:
//
//   - proven.Returns wraps a return value, advertising a function-
//     level postcondition that every caller across packages sees
//     via the preprocessor's sidecar.
//   - trust.That is local — it injects facts into the enclosing
//     function's flow state, invisible to callers. If every
//     caller should see the fact, use proven.Returns instead.
//
// Under the preprocessor, trust.That call sites are erased and
// the facts flow into the analyzer's per-caller fact set. Without
// the preprocessor the call is a plain pass-through: a program
// that uses only trust (no proven.That / proven.Returns) links
// and runs without the toolexec pipeline, treating trust.That as
// identity.
package trust

// That asserts that every predicate holds on v and returns v
// unchanged. No runtime verification is performed; see the
// package doc for the contract and when to reach for it.
func That[T any](v T, preds ...func(T) bool) T {
	_ = preds
	return v
}

// Returns is the "trust me" variant of proven.Returns: it
// combines trust.That's no-runtime-check honor-system semantics
// with proven.Returns's function-level postcondition advertisement.
// The enclosing function's callers see v as carrying every listed
// predicate as a fact, exactly as if the function returned
// `proven.Returns(v, preds...)` — but without the verification the
// preprocessor applies at a proven.Returns call site.
//
// Use in the same situations as trust.That: the value is known
// to satisfy the predicates through some mechanism the analyzer
// cannot see, and a runtime re-check would duplicate earlier
// validation. Prefer proven.Returns over a local variable plus a
// guard whenever the analyzer can prove the value locally;
// trust.Returns is the escape hatch for literals, computed
// expressions, and other values the flow analyzer cannot reason
// about but the programmer knows are sound.
//
// Typical shape:
//
//	func DefaultUserID() int {
//	    return trust.Returns(42, isPositive) // 42 obviously > 0
//	}
//
// Under the preprocessor the call is erased the same way as
// proven.Returns and trust.That: the wrapper disappears, v
// survives at its original column, and each predicate is added to
// the enclosing function's ReturnPreds summary so downstream
// callers discharge without re-proving.
func Returns[T any](v T, preds ...func(T) bool) T {
	_ = preds
	return v
}
