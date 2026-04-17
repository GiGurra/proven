// Package inferenceexperiment probes Go's generic type-inference behavior
// for the shapes of "wrap this raw value into a proven.Refined" we are
// considering for public API. The point is to document which call-site
// forms actually work — so we can decide between `proven.Attest[P](x)`,
// `proven.In[P](x)`, and a context-inferred `proven.In(x)`.
//
// All runtime behavior here is stubbed (returns zeros). Only the compile
// step is meaningful: if the file compiles, the inference worked.
package inferenceexperiment
