package preprocessor

// Phase 3 of the demand-driven restructure: event-stream resolver.
//
// Each of these methods answers a question about the analyzer's
// current flow state — "does pred hold on v?", "is an Or-obligation
// satisfied?", "which facts hold on v at this return?" — by walking
// the event stream backward from the current emission point.
// Visibility across scopes uses ancestor-or-self: an event is
// visible to the point-of-query iff its scope lies on the path from
// root to the query's scope. A Write on v in a visible scope is a
// barrier: any earlier Source on v is no longer reachable once the
// backward walk crosses it.
//
// The resolver replaces the mutable-FactSet read paths; fact-mutation
// still happens in the walk (for side-by-side validation during
// rollout) but the discharge decision no longer consults it.

import "slices"

// hasFactViaEvents reports whether pred holds on varName at the
// current emission point. It walks the recorded events backward from
// the most recent one, stopping at the first visible Source on
// (pred, varName) — returning true — or the first visible Write on
// varName's root — returning false. Entries in non-ancestor scopes
// are skipped: a Source in a sibling branch is invisible, matching
// the original analyzer's clone-and-restore semantics.
//
// Write-barrier matching respects the canonical-key rooting: a Write
// records only the root identifier (e.g. "holder" for a `holder.F =
// v` LHS), and any query whose canonical key starts at that root is
// invalidated. So a Write on "holder" blocks "holder", "holder.Value",
// "holder.A.B", and so on — preserving the Phase-1 forgetLHS
// semantics under canonical expression keys.
//
// The method assumes the analyzer has just emitted a Query event for
// (pred, varName) and is asking about visibility at THAT event's
// position; the Query itself is skipped during the backward walk.
func (a *analyzer) hasFactViaEvents(pred Predicate, varName string) bool {
	if varName == "" {
		return false
	}
	events := a.rec.events
	if len(events) == 0 {
		return false
	}
	root := exprKeyRoot(varName)
	qScope := events[len(events)-1].Scope
	for i := len(events) - 2; i >= 0; i-- {
		ev := events[i]
		if !a.rec.tree.isAncestorOrSelf(ev.Scope, qScope) {
			continue
		}
		if ev.Kind == evWrite && ev.Var == root {
			return false
		}
		if ev.Kind == evSourceLeaf && ev.Pred == pred && ev.Var == varName {
			return true
		}
	}
	return false
}

// hasOrFactViaEvents reports whether an exact-match disjunctive fact
// (alt-order-insensitive) holds on varName at the current emission
// point. Mirrors hasFactViaEvents's walk; matches SourceOr events by
// orKey.
func (a *analyzer) hasOrFactViaEvents(alts []Predicate, varName string) bool {
	if varName == "" || len(alts) == 0 {
		return false
	}
	events := a.rec.events
	if len(events) == 0 {
		return false
	}
	key := orKey(alts)
	root := exprKeyRoot(varName)
	qScope := events[len(events)-1].Scope
	for i := len(events) - 2; i >= 0; i-- {
		ev := events[i]
		if !a.rec.tree.isAncestorOrSelf(ev.Scope, qScope) {
			continue
		}
		if ev.Kind == evWrite && ev.Var == root {
			return false
		}
		if ev.Kind == evSourceOr && ev.Var == varName && orKey(ev.Alts) == key {
			return true
		}
	}
	return false
}

// dischargedViaEvents reports whether pred holds on varName either
// directly (hasFactViaEvents) or by inference through declared rules.
// Cycle-safe: visited collects every predicate the query has already
// attempted on varName so degenerate cycles terminate.
func (a *analyzer) dischargedViaEvents(pred Predicate, varName string) bool {
	if varName == "" {
		return false
	}
	return a.dischargedViaEventsRec(pred, varName, make(map[Predicate]bool))
}

func (a *analyzer) dischargedViaEventsRec(pred Predicate, varName string, visited map[Predicate]bool) bool {
	if a.hasFactViaEvents(pred, varName) {
		return true
	}
	if visited[pred] {
		return false
	}
	visited[pred] = true
	for _, rule := range a.allRules() {
		if !slices.Contains(rule.To, pred) {
			continue
		}
		if !a.allDischargeViaEvents(rule.From, varName, visited) {
			continue
		}
		if len(rule.Given) > 0 && !a.allDischargeViaEvents(rule.Given, varName, visited) {
			continue
		}
		return true
	}
	return false
}

func (a *analyzer) allDischargeViaEvents(preds []Predicate, varName string, visited map[Predicate]bool) bool {
	for _, p := range preds {
		if !a.dischargedViaEventsRec(p, varName, visited) {
			return false
		}
	}
	return true
}

// dischargedOrViaEvents reports whether a disjunctive obligation
// holds on varName at the current emission point. Two paths succeed:
//
//   - Any alt in alts is dischargedViaEvents on varName (a stronger
//     fact implies the Or).
//   - A structural-equality Or-fact is recorded on varName.
//
// An empty alts list never discharges.
func (a *analyzer) dischargedOrViaEvents(alts []Predicate, varName string) bool {
	if varName == "" || len(alts) == 0 {
		return false
	}
	for _, p := range alts {
		if a.dischargedViaEvents(p, varName) {
			return true
		}
	}
	return a.hasOrFactViaEvents(alts, varName)
}

// snapshotFactsOnVarViaEvents collects the leaf predicates and Or-alt
// lists that hold on varName at the current emission point. Used by
// snapshotReturn to produce the fact set on a returned identifier
// for DerivedReturnPreds / DerivedReturnOrs intersection.
//
// The walk is backward from the most recent event until the first
// visible Write on varName (exclusive) or function entry. Duplicate
// leaf preds and duplicate Or alt sets (by canonical key) are
// collapsed so the snapshot contains each distinct fact once.
func (a *analyzer) snapshotFactsOnVarViaEvents(varName string) ([]Predicate, [][]Predicate) {
	if varName == "" {
		return nil, nil
	}
	events := a.rec.events
	if len(events) == 0 {
		return nil, nil
	}
	root := exprKeyRoot(varName)
	qScope := a.rec.current
	seen := make(map[Predicate]struct{})
	seenOrs := make(map[string]struct{})
	var leaves []Predicate
	var ors [][]Predicate
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if !a.rec.tree.isAncestorOrSelf(ev.Scope, qScope) {
			continue
		}
		if ev.Kind == evWrite && ev.Var == root {
			break
		}
		if ev.Var != varName {
			continue
		}
		switch ev.Kind {
		case evSourceLeaf:
			if _, dup := seen[ev.Pred]; !dup {
				seen[ev.Pred] = struct{}{}
				leaves = append(leaves, ev.Pred)
			}
		case evSourceOr:
			k := orKey(ev.Alts)
			if _, dup := seenOrs[k]; !dup {
				seenOrs[k] = struct{}{}
				ors = append(ors, append([]Predicate(nil), ev.Alts...))
			}
		}
	}
	return leaves, ors
}
