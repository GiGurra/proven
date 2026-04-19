package preprocessor

import (
	"reflect"
	"testing"
)

func TestParseChain_Classic(t *testing.T) {
	_, chain, tool, toolArgs, ok := parseChain([]string{
		"/goroot/pkg/tool/linux_amd64/compile", "-p", "foo", "a.go",
	})
	if !ok {
		t.Fatalf("parse failed")
	}
	if len(chain.NextCmd) != 0 {
		t.Errorf("expected no chain, got %v", chain.NextCmd)
	}
	if tool != "/goroot/pkg/tool/linux_amd64/compile" {
		t.Errorf("tool=%q", tool)
	}
	if !reflect.DeepEqual(toolArgs, []string{"-p", "foo", "a.go"}) {
		t.Errorf("toolArgs=%v", toolArgs)
	}
}

func TestParseChain_OneHop(t *testing.T) {
	args := []string{
		"--and-then", "rewire",
		"/goroot/pkg/tool/linux_amd64/compile", "-p", "foo", "a.go",
	}
	_, chain, tool, toolArgs, ok := parseChain(args)
	if !ok {
		t.Fatalf("parse failed")
	}
	if !reflect.DeepEqual(chain.NextCmd, []string{"rewire"}) {
		t.Errorf("chain=%v", chain.NextCmd)
	}
	if tool != "/goroot/pkg/tool/linux_amd64/compile" {
		t.Errorf("tool=%q", tool)
	}
	if !reflect.DeepEqual(toolArgs, []string{"-p", "foo", "a.go"}) {
		t.Errorf("toolArgs=%v", toolArgs)
	}
}

func TestParseChain_MultiHop(t *testing.T) {
	args := []string{
		"--proven-flag",
		"--and-then", "rewire", "--rewire-flag",
		"--and-then", "third", "--third-flag",
		"/goroot/pkg/tool/linux_amd64/compile", "-p", "foo", "a.go",
	}
	provenArgs, chain, tool, toolArgs, ok := parseChain(args)
	if !ok {
		t.Fatalf("parse failed")
	}
	if !reflect.DeepEqual(provenArgs, []string{"--proven-flag"}) {
		t.Errorf("provenArgs=%v", provenArgs)
	}
	wantChain := []string{"rewire", "--rewire-flag", "--and-then", "third", "--third-flag"}
	if !reflect.DeepEqual(chain.NextCmd, wantChain) {
		t.Errorf("chain=%v want=%v", chain.NextCmd, wantChain)
	}
	if tool != "/goroot/pkg/tool/linux_amd64/compile" {
		t.Errorf("tool=%q", tool)
	}
	if !reflect.DeepEqual(toolArgs, []string{"-p", "foo", "a.go"}) {
		t.Errorf("toolArgs=%v", toolArgs)
	}
}

func TestParseChain_AsmTool(t *testing.T) {
	args := []string{
		"--and-then", "rewire",
		"/goroot/pkg/tool/linux_amd64/asm", "input.s",
	}
	_, chain, tool, toolArgs, ok := parseChain(args)
	if !ok {
		t.Fatalf("parse failed")
	}
	if !reflect.DeepEqual(chain.NextCmd, []string{"rewire"}) {
		t.Errorf("chain=%v", chain.NextCmd)
	}
	if tool != "/goroot/pkg/tool/linux_amd64/asm" {
		t.Errorf("tool=%q", tool)
	}
	if !reflect.DeepEqual(toolArgs, []string{"input.s"}) {
		t.Errorf("toolArgs=%v", toolArgs)
	}
}

func TestParseChain_AbsolutePrePath(t *testing.T) {
	// Preprocessor binary passed as absolute path (not just name).
	// The go-tool locator prefers known tool bases, so /usr/local/bin/rewire
	// is skipped and the real compile tool is found.
	args := []string{
		"--and-then", "/usr/local/bin/rewire", "--flag",
		"/goroot/pkg/tool/linux_amd64/compile", "-p", "foo",
	}
	_, chain, tool, _, ok := parseChain(args)
	if !ok {
		t.Fatalf("parse failed")
	}
	if !reflect.DeepEqual(chain.NextCmd, []string{"/usr/local/bin/rewire", "--flag"}) {
		t.Errorf("chain=%v", chain.NextCmd)
	}
	if tool != "/goroot/pkg/tool/linux_amd64/compile" {
		t.Errorf("tool=%q", tool)
	}
}

func TestParseChain_ExeSuffix(t *testing.T) {
	args := []string{
		"--and-then", "rewire",
		"/goroot/pkg/tool/linux_amd64/compile.exe", "-p", "foo",
	}
	_, _, tool, _, ok := parseChain(args)
	if !ok {
		t.Fatalf("parse failed")
	}
	if tool != "/goroot/pkg/tool/linux_amd64/compile.exe" {
		t.Errorf("tool=%q", tool)
	}
}

func TestParseChain_NoToolAfterAndThen(t *testing.T) {
	args := []string{"--and-then", "rewire", "--flag"}
	_, _, _, _, ok := parseChain(args)
	if ok {
		t.Fatalf("expected parse failure for chain with no tool")
	}
}
