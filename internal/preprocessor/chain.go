package preprocessor

import (
	"path/filepath"
	"strings"
)

// Chain captures the optional `--and-then` chain of successor
// preprocessor invocations. A non-empty NextCmd means proven was
// invoked as one link in a chain like:
//
//	proven [proven-args...] --and-then pre2 [pre2-args...] --and-then pre3 ... /abs/go/tool args...
//
// When the chain is active, proven still does its normal preprocessing
// work, but instead of exec'ing the real Go tool directly it exec's
// NextCmd with the tool path (and possibly-rewritten tool args)
// appended. Each link in the chain peels off one --and-then and
// forwards the remainder, so the last link sees a classic toolexec
// invocation and needs no --and-then awareness at all.
type Chain struct {
	// NextCmd is the argv that should replace the bare tool
	// invocation. Empty when no --and-then was present.
	NextCmd []string
}

// parseChain splits the proven toolexec args (i.e. os.Args[1:]) into
// its components:
//
//	provenArgs — flags intended for proven itself (reserved for future use; empty for now)
//	chain      — the successor command, if --and-then was used
//	toolPath   — the absolute path of the Go tool proven wraps
//	toolArgs   — the tool's own arguments
//
// Without --and-then the parse is the classic toolexec shape:
// args[0] is the tool, args[1:] its arguments. With --and-then,
// proven consumes everything up to the first --and-then as its own
// args, then scans forward to locate the Go tool (by absolute path
// + known tool base name). Everything between the --and-then and
// the Go tool is the successor argv.
//
// ok is false when args look like a broken chain invocation — e.g.
// a --and-then with no identifiable Go tool after it.
func parseChain(args []string) (provenArgs []string, chain Chain, toolPath string, toolArgs []string, ok bool) {
	split := -1
	for i, a := range args {
		if a == "--and-then" {
			split = i
			break
		}
	}
	if split == -1 {
		if len(args) == 0 {
			return nil, Chain{}, "", nil, false
		}
		return nil, Chain{}, args[0], args[1:], true
	}
	provenArgs = args[:split]
	rest := args[split+1:]
	toolIdx := findGoToolIndex(rest)
	if toolIdx < 0 {
		return provenArgs, Chain{}, "", nil, false
	}
	chain = Chain{NextCmd: rest[:toolIdx]}
	toolPath = rest[toolIdx]
	toolArgs = rest[toolIdx+1:]
	return provenArgs, chain, toolPath, toolArgs, true
}

// goToolBases is the set of basename identifiers Go's toolchain uses
// for the tools invoked via -toolexec. We use this to locate the
// real Go tool in a chain argv: the first absolute-path arg whose
// basename is in this set is the tool, and anything before it is
// successor-preprocessor metadata.
var goToolBases = map[string]bool{
	"compile":   true,
	"link":      true,
	"asm":       true,
	"cgo":       true,
	"vet":       true,
	"nm":        true,
	"objdump":   true,
	"pack":      true,
	"buildid":   true,
	"addr2line": true,
	"covdata":   true,
	"test2json": true,
	"trace":     true,
	"fix":       true,
}

func findGoToolIndex(args []string) int {
	for i, a := range args {
		if !filepath.IsAbs(a) {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(a), ".exe")
		if goToolBases[base] {
			return i
		}
	}
	return -1
}
