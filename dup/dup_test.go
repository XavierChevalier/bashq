package dup

import (
	"fmt"
	"strings"
	"testing"

	"github.com/XavierChevalier/bashq/metrics"
)

func source(t *testing.T, path, src string) Source {
	t.Helper()
	tree, err := metrics.Parse(path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return Source{Path: path, Tree: tree}
}

// body is twelve lines and twelve units of code that only a copy matches.
func body(prefix string) string {
	var b strings.Builder
	b.WriteString("if [[ -d $dir ]]; then\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "  %s_%d \"$dir\" --flag=%d\n", prefix, i, i)
	}
	b.WriteString("fi\n")
	return b.String()
}

func TestFindsACopyAcrossFiles(t *testing.T) {
	a := source(t, "a.sh", "a() {\n"+body("step")+"}\n")
	// Same code, other layout and comments: still a copy.
	b := source(t, "b.sh", "echo start\n# copied\nb() {\n"+strings.ReplaceAll(body("step"), "  ", "\t")+"}\n")
	blocks := Find([]Source{a, b}, Options{MinUnits: 10, MinLines: 10})
	if len(blocks) != 1 {
		t.Fatalf("blocks = %+v, want 1", blocks)
	}
	got := blocks[0]
	if got.First != (Range{"a.sh", 1, 13}) || got.Second != (Range{"b.sh", 3, 15}) {
		t.Errorf("block = %+v", got)
	}
	if got.Units != 15 {
		t.Errorf("units = %d, want 15: the function header (names differ), if, the test, then, 10 commands, fi", got.Units)
	}
	lines := DuplicatedLines(blocks)
	if lines["a.sh"] != 13 || lines["b.sh"] != 13 {
		t.Errorf("duplicated lines = %v", lines)
	}
}

func TestDifferentCodeIsNotACopy(t *testing.T) {
	a := source(t, "a.sh", body("step"))
	b := source(t, "b.sh", body("other"))
	if blocks := Find([]Source{a, b}, Options{MinUnits: 10, MinLines: 10}); len(blocks) != 0 {
		t.Errorf("blocks = %+v, want none", blocks)
	}
}

func TestShortCopiesAreIgnored(t *testing.T) {
	a := source(t, "a.sh", body("step"))
	b := source(t, "b.sh", body("step"))
	if blocks := Find([]Source{a, b}, Options{MinUnits: 10, MinLines: 20}); len(blocks) != 0 {
		t.Errorf("blocks = %+v, want none under 20 lines", blocks)
	}
	if blocks := Find([]Source{a, b}, Options{MinUnits: 20, MinLines: 10}); len(blocks) != 0 {
		t.Errorf("blocks = %+v, want none under 20 units", blocks)
	}
}

func TestCopyInTheSameFile(t *testing.T) {
	a := source(t, "a.sh", body("step")+"echo between\n"+body("step"))
	blocks := Find([]Source{a}, Options{MinUnits: 10, MinLines: 10})
	if len(blocks) != 1 || blocks[0].First.StartLine != 1 || blocks[0].Second.StartLine != 14 {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestRepeatedLinesAreNotACopyOfThemselves(t *testing.T) {
	a := source(t, "a.sh", strings.Repeat("echo same\n", 40))
	blocks := Find([]Source{a}, Options{MinUnits: 10, MinLines: 10})
	for _, b := range blocks {
		if b.First.EndLine >= b.Second.StartLine {
			t.Errorf("overlapping copies: %+v", b)
		}
	}
}

func TestThreeCopies(t *testing.T) {
	var sources []Source
	for _, name := range []string{"a.sh", "b.sh", "c.sh"} {
		sources = append(sources, source(t, name, body("step")))
	}
	blocks := Find(sources, Options{MinUnits: 10, MinLines: 10})
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want one per pair of copies", len(blocks))
	}
	if lines := DuplicatedLines(blocks); lines["c.sh"] != 12 {
		t.Errorf("c.sh duplicated lines = %d, want 12", lines["c.sh"])
	}
}
