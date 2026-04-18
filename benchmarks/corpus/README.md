# benchmarks/corpus

A synthetic but realistic Go module used to measure preprocessor performance and surface corner-case bugs the hand-written e2e fixtures under `testdata/cases/` do not.

## Layout

```
benchmarks/corpus/
    go.mod                   # own module with `replace` back to the parent repo
    preds/                   # shared predicates imported by many leaf packages
    rules/                   # shared infer.From(...).To(...) rules
    leaf/l00..l09/           # annotated packages, no imports of other corpus pkgs
    mid/m00..m09/            # import ~2 leaves, exercise guard / Returns / infer discharge
    high/h00..h04/           # complex callers across leaf and mid
    bound/b00..b02/          # prove.That / prove.Must / trust.That flows
    rel/                     # tuple-subject relation pattern
    cmd/main/                # driver that touches every package so `go build ./...` links it all
```

The corpus is deliberately handwritten — not generated — so new patterns can be added one at a time when we want to exercise a specific behavior.

## Running it

**Install the preprocessor binary first** (the bench runner shells out to it):

```
go install ./cmd/proven
```

**Build the whole corpus under toolexec:**

```
cd benchmarks/corpus
GOCACHE=$(mktemp -d) go build -toolexec="$(go env GOPATH)/bin/proven" ./...
```

Use an isolated `GOCACHE` when mixing toolexec-on and toolexec-off flows — Go's build cache key does not include toolexec behavior, so a `.a` from one flow will be silently reused by the other and the link will fail in confusing ways. The bench runner under `benchmarks/bench/` handles this automatically.

## What the corpus exercises today

- `proven.That` single-predicate obligations on int, string, and slice subjects.
- `proven.That` multi-predicate (AND-composed) obligations.
- `proven.Returns` postconditions flowing between functions.
- Discharge via preceding `if pred(x)` guards.
- Discharge via early-return guards (`if !pred(x) { return }`).
- Discharge via `&&`-conjoined guards.
- Discharge via `proven.Returns` postconditions.
- Discharge via `prove.That` success branches.
- Discharge via `prove.Must`.
- Local fact injection via `trust.That`.
- Cross-package obligations via sidecars (leaf → mid → high imports).
- Declared inference rules in a shared `rules/` package consumed across the import graph.
- Method obligations with both value and pointer receivers.
- Package-local (unexported) predicates resolved via cross-package selectors once exported.
- Tuple-subject relations (the Phase 7.5 pattern).
- A `cmd/main/` driver that links everything so build failures are loud.

## Growing the corpus

When a preprocessor change is landing or a new pattern surfaces, add a new package — a new `leaf/lNN/` for a leaf-layer pattern, a new `mid/mNN/` for cross-package patterns, or a new top-level directory for an entirely new category. Import it from `cmd/main/main.go` so `go build ./...` continues to pull the full graph.

A good test pattern is: **write the pattern as user code you wish worked, run the toolexec build, and if the preprocessor chokes, fix the preprocessor and keep the package**. The two bugs surfaced by the initial corpus (rewriter leaving dangling user imports; sidecar dropped for rules-only packages) were both found this way.
