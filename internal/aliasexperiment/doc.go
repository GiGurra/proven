// Package aliasexperiment probes whether Go's generic type aliases can
// carry phantom proof metadata invisible to the type checker.
//
// The idea: declare `type Where[T any, P any] = T`. The Go compiler
// resolves Where[int, Positive] to int. No wrapping, no conversions.
// Raw ints flow through. The preprocessor — reading source, not
// resolved types — sees the Where[int, P] shape in signatures and
// enforces P as a proof obligation.
//
// If this compiles and behaves as expected, it changes the design
// entirely. Functions expose ordinary Go signatures (gopls sees int),
// while the source retains the metadata needed for preprocessor checks.
package aliasexperiment
