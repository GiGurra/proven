// Same package shape as cross_package_ok/callee, but with the
// caller lacking a discharge: the preprocessor must surface the
// cross-package undischarged-obligation diagnostic.

package callee

import "github.com/GiGurra/proven/pkg/proven"

func IsPositive(x int) bool { return x > 0 }

func Target(amount int) {
	proven.That(amount, IsPositive)
}
