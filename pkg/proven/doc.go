// Package proven provides compile-time contracts for Go. Functions
// declare preconditions and postconditions in their body via
// proven.That and proven.Returns; the proven preprocessor discharges
// them at build time.
//
// # Preprocessor is required at link time, not at type-check time
//
// proven.That and proven.Returns wrap their checks in a package-private
// atCompileTime helper. atCompileTime is declared via //go:linkname to
// an external symbol, _proven_atCompileTime, with no Go body. The proven
// preprocessor supplies this symbol during the toolexec pass. The
// consequence:
//
//   - IDEs, gopls, and `go vet` see ordinary Go. No red squiggles, no
//     configuration, no build-tag ceremony. Type-checking is always green.
//   - `go build` / `go test` / any link step fails without the
//     preprocessor, with an error like
//     "relocation target _proven_atCompileTime not defined".
//   - With the preprocessor active (GOFLAGS="-toolexec=proven"), the
//     preprocessor injects the missing symbol, link succeeds, and
//     obligations are discharged statically.
//
// This keeps the in-editor experience pristine while making
// production-like builds refuse to succeed without the preprocessor.
//
// See docs/design.md and docs/companion-packages.md for the full design.
package proven
