// Package embeddingexperiment probes the (C) struct-embedding approach
// for attaching proof markers to values. Goal: verify that the patterns
// we want to adopt — embedded predicate markers, primitive wrappers,
// multi-predicate composition, flow-through via return values — compile
// cleanly in Go 1.26 and are pleasant to write.
//
// Runtime behavior is minimal here. Only compile-level behavior and
// method resolution are being exercised.
package embeddingexperiment
