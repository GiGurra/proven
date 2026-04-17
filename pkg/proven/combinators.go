package proven

// And[P1, P2] is satisfied when both P1 and P2 are satisfied.
type And[P1, P2 any] struct{}

// Or[P1, P2] is satisfied when either P1 or P2 is satisfied.
type Or[P1, P2 any] struct{}

// Not[P] is satisfied when P is not satisfied.
type Not[P any] struct{}

// Combinators are phantom types — no runtime methods are provided. The
// preprocessor evaluates them by expanding to the component predicates'
// Check calls when runtime evaluation is needed (TrustMe). For user-defined
// compound predicates with a single Check method, prefer a named struct.
