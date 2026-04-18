// NonEmptyMap used as a guard predicate works the same way as
// NonEmptySlice — generic over the K / V parameters, keyed by its
// predicate identity in the analyzer. The downstream call
// discharges from the guard-planted fact.

package main

import "github.com/GiGurra/proven/pkg/proven"

func process(m map[string]int) {
	proven.That(m, proven.NonEmptyMap)
	_ = m["a"]
}

func run(m map[string]int) {
	if proven.NonEmptyMap(m) {
		process(m)
	}
}

func main() {
	run(map[string]int{"a": 1})
}
