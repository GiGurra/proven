package preprocessor

// Phase 1 of the demand-driven analyzer restructure: event data
// model and recorder.
//
// Each fact-relevant action during the analyzer's walk — establishing
// a fact, mutating a tracked variable, querying a fact at a call site
// — is recorded as an Event with a position and a scope. Subsequent
// phases replace the eager FactSet-based resolver with on-demand
// backward search over this stream; at the initial roll-in, events
// are recorded passively alongside the existing FactSet so the
// stream's shape can be validated against a known-good baseline
// before any resolution logic is rewired.
//
// Scope tracking: each block / branch / loop body that the existing
// analyzer treats as a clone-and-restore barrier opens its own scope
// node, whose parent is the enclosing scope. The resolver uses an
// ancestor-or-self check to decide whether a source event is visible
// to a query — branch siblings are invisible to each other, but every
// enclosing ancestor is visible down into its children.

import "go/token"

// eventKind labels the role of an Event in the fact stream.
type eventKind int

const (
	evSourceLeaf eventKind = iota + 1 // pred holds on v at pos
	evSourceOr                        // one of alts holds on v at pos
	evWrite                           // v was mutated at pos (barrier)
	evQueryLeaf                       // prove pred on v at pos
	evQueryOr                         // prove any of alts on v at pos
)

// Event is one entry in the recorded fact stream. The semantics of
// each field depend on kind:
//
//   evSourceLeaf: Pred holds on Var at Pos inside Scope.
//   evSourceOr:   One of Alts holds on Var at Pos inside Scope.
//   evWrite:      Var was mutated at Pos inside Scope. Any Source
//                 event on Var in an ancestor-or-self scope preceding
//                 this Write is no longer visible to later queries.
//   evQueryLeaf:  A call-site obligation that Pred hold on Var at Pos.
//                 The resolver decides whether any reachable source
//                 discharges it.
//   evQueryOr:    Same, but the obligation is disjunctive over Alts.
//
// Var in Phase 1 is always a bare identifier name; Phase 2 widens it
// to a canonical expression key that includes selector paths.
type Event struct {
	Kind  eventKind
	Pos   token.Pos
	Scope int
	Pred  Predicate
	Alts  []Predicate
	Var   string

	// DischargeIdx / ParamIdx stitch query events back into the
	// CallDischarge records the analyzer emits: each query's
	// resolution result updates the Missing / MissingOrs slices of
	// the corresponding parameter discharge.
	DischargeIdx int
	ParamIdx     int
}

// scopeTree captures the lexical nesting of analyzer walk positions.
// Each new block / branch / loop body that the analyzer previously
// marked with Clone/restore opens a scope whose parent is the
// enclosing one. Scope 0 is the function-root scope; it has no
// parent (parent id -1).
type scopeTree struct {
	parents []int
}

func newScopeTree() *scopeTree {
	return &scopeTree{parents: []int{-1}}
}

// open allocates a new scope id as a child of parent and returns it.
func (s *scopeTree) open(parent int) int {
	id := len(s.parents)
	s.parents = append(s.parents, parent)
	return id
}

// parentOf returns scope's parent id, or -1 for the root / unknown.
func (s *scopeTree) parentOf(id int) int {
	if id < 0 || id >= len(s.parents) {
		return -1
	}
	return s.parents[id]
}

// isAncestorOrSelf reports whether a is an ancestor of b, or equal
// to it. An event in scope a is visible to a query in scope b iff
// a lies on the path from root to b.
func (s *scopeTree) isAncestorOrSelf(a, b int) bool {
	for b >= 0 {
		if a == b {
			return true
		}
		b = s.parentOf(b)
	}
	return false
}

// eventRecorder accumulates the Event stream for one function's
// analyzer walk. The analyzer instantiates a fresh recorder per
// function; the recorded events are scoped to that function only.
type eventRecorder struct {
	tree    *scopeTree
	events  []Event
	current int // current scope during the walk
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{tree: newScopeTree()}
}

// sourceLeaf records that pred holds on v at pos in the current
// scope. A blank v (no identifier) is dropped: the fact-set semantic
// already treats nameless subjects as un-trackable.
func (r *eventRecorder) sourceLeaf(pos token.Pos, pred Predicate, v string) {
	if v == "" {
		return
	}
	r.events = append(r.events, Event{
		Kind:  evSourceLeaf,
		Pos:   pos,
		Scope: r.current,
		Pred:  pred,
		Var:   v,
	})
}

// sourceOr records that one of alts holds on v at pos in the current
// scope. An empty alts list or blank v is dropped for the same reason
// the FactSet.AddOr bailout drops them: nothing to record.
func (r *eventRecorder) sourceOr(pos token.Pos, alts []Predicate, v string) {
	if v == "" || len(alts) == 0 {
		return
	}
	r.events = append(r.events, Event{
		Kind:  evSourceOr,
		Pos:   pos,
		Scope: r.current,
		Alts:  append([]Predicate(nil), alts...),
		Var:   v,
	})
}

// write records a mutation to v at pos. In Phase 3 resolution, this
// is a barrier: a Source on v in an ancestor-or-self scope preceding
// this event is no longer visible to queries following it.
func (r *eventRecorder) write(pos token.Pos, v string) {
	if v == "" {
		return
	}
	r.events = append(r.events, Event{
		Kind:  evWrite,
		Pos:   pos,
		Scope: r.current,
		Var:   v,
	})
}

// queryLeaf records a leaf obligation at pos. dischargeIdx and
// paramIdx are opaque to the recorder; the analyzer uses them to
// route the resolver's verdict back into the CallDischarge record
// being built.
func (r *eventRecorder) queryLeaf(pos token.Pos, pred Predicate, v string, dischargeIdx, paramIdx int) {
	r.events = append(r.events, Event{
		Kind:         evQueryLeaf,
		Pos:          pos,
		Scope:        r.current,
		Pred:         pred,
		Var:          v,
		DischargeIdx: dischargeIdx,
		ParamIdx:     paramIdx,
	})
}

// queryOr records a disjunctive obligation at pos.
func (r *eventRecorder) queryOr(pos token.Pos, alts []Predicate, v string, dischargeIdx, paramIdx int) {
	r.events = append(r.events, Event{
		Kind:         evQueryOr,
		Pos:          pos,
		Scope:        r.current,
		Alts:         append([]Predicate(nil), alts...),
		Var:          v,
		DischargeIdx: dischargeIdx,
		ParamIdx:     paramIdx,
	})
}

// enterScope opens a fresh scope under the current one and makes it
// current. Returns the prior scope id so the caller can restore it
// with leaveScope.
func (r *eventRecorder) enterScope() int {
	prev := r.current
	r.current = r.tree.open(prev)
	return prev
}

// leaveScope restores the current scope to prev. Callers must pair
// enterScope / leaveScope strictly; a missed leaveScope leaves later
// events stranded inside a branch scope and corrupts visibility.
func (r *eventRecorder) leaveScope(prev int) {
	r.current = prev
}
