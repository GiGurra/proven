// Command main ties the whole corpus together into a buildable
// main package so `go build -toolexec=proven ./...` reaches every
// leaf, mid, high, boundary, and relation package.
package main

import (
	"fmt"

	"github.com/GiGurra/proven/benchmarks/corpus/bound/b00"
	"github.com/GiGurra/proven/benchmarks/corpus/bound/b01"
	"github.com/GiGurra/proven/benchmarks/corpus/bound/b02"
	"github.com/GiGurra/proven/benchmarks/corpus/high/h00"
	"github.com/GiGurra/proven/benchmarks/corpus/high/h01"
	"github.com/GiGurra/proven/benchmarks/corpus/high/h02"
	"github.com/GiGurra/proven/benchmarks/corpus/high/h03"
	"github.com/GiGurra/proven/benchmarks/corpus/high/h04"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m01"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m02"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m04"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m05"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m06"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m07"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m08"
	"github.com/GiGurra/proven/benchmarks/corpus/mid/m09"
	"github.com/GiGurra/proven/benchmarks/corpus/rel"
)

func main() {
	// Drive every exported function just enough to keep the linker
	// happy; the value of this main is that it forces the whole
	// graph to link, not that it does useful work.
	_ = h00.Pipeline(5)
	_ = h01.ManyCalls(1, 2, 3, 4)
	_ = h01.NormalizeAndTarget(0)
	_ = h02.DeepInference(10)
	_, _ = h03.Orchestrate(8, 2)
	_ = h04.Process("hi", 3)
	h04.FanOut("hello")

	_ = m01.CallBoth(50)
	_ = m01.NegateGuard(-1)
	_ = m02.Normalize(2)
	_ = m02.Double(2)
	_ = m04.CountFrom(0, 3)
	_ = m05.FillBucket(16, 4)
	m06.Ingest("data")
	m06.Strict("data")
	m07.Feed(30)
	m07.FeedTriple(9)
	_ = m08.CallPair(1, 0)
	_ = m08.CallTriple(1, 2, 3)
	_ = m08.CallMixed("x", 1)
	_ = m09.Head([]int{1, 2, 3})
	_ = m09.Search([]int{1, 2, 3}, 2)

	if v, err := b00.Accept(1); err == nil {
		fmt.Println(v)
	}
	fmt.Println(b00.MustAccept(1))
	fmt.Println(b01.Forward(1))
	fmt.Println(b01.ForwardCompound(5))
	if err := b02.HandleString("hello"); err != nil {
		fmt.Println(err)
	}
	b02.StartupString("payload")

	rel.Invoke(rel.Session{ID: "s"}, rel.User{ID: "u"}, rel.Resource{ID: "r"})
}
