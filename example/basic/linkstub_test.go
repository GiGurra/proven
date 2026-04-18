package basic

// This file exists only so this example package's TEST binary can link
// in the absence of the proven preprocessor (which is not yet built).
// It supplies a no-op stub for the _proven_atCompileTime symbol that
// proven.That / proven.Returns reference via //go:linkname.
//
// Because the file is *_test.go, it is included only when building a
// test binary for this package — production builds still refuse to
// link without the preprocessor. Downstream users adopting proven
// before the preprocessor ships would need their own equivalent stub.

import _ "unsafe" // for //go:linkname

//go:linkname _proven_atCompileTime _proven_atCompileTime
func _proven_atCompileTime(_ func()) {}
