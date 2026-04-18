// Package rules holds inference rules used by many packages in
// the corpus. Declaring them in a single shared package exercises
// the Phase 6 sidecar import path that lets downstream packages
// pick rules up via -importcfg.
package rules

import (
	"github.com/GiGurra/proven/benchmarks/corpus/preds"
	"github.com/GiGurra/proven/pkg/infer"
)

var _ = infer.From(preds.IsSmallPositive).To(preds.IsPositive)
var _ = infer.From(preds.IsSmallPositive).To(preds.IsSmall)
var _ = infer.From(preds.IsSmallPositive).To(preds.IsNonNeg)
var _ = infer.From(preds.IsPositive).To(preds.IsNonNeg)
var _ = infer.From(preds.IsEven).Given(preds.IsPositive).To(preds.IsNonNeg)
var _ = infer.From(preds.IsInByteRange).To(preds.IsNonNeg)
var _ = infer.From(preds.IsInByteRange).To(preds.IsSmall)
var _ = infer.From(preds.IsLarge).To(preds.IsPositive)
var _ = infer.From(preds.IsMidRange).To(preds.IsPositive)
var _ = infer.From(preds.IsMidRange).To(preds.IsNonNeg)
