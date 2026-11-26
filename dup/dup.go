// Package dup finds duplicated code across bash scripts.
//
// Every script becomes a sequence of units: one per simple command, printed
// back in shfmt's canonical form (so spacing, quoting style of the layout and
// comments do not matter), and one per keyword of a compound command (if,
// then, else, fi, for, do, done, case, a case pattern, esac, a function
// header). Two runs of at least MinUnits equal units, spanning at least
// MinLines lines, are a duplicated block.
package dup

import (
	"bytes"
	"hash/fnv"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Options tunes the detection.
type Options struct {
	// MinUnits is the smallest number of equal consecutive units (commands
	// and keywords) reported. Default 15.
	MinUnits int
	// MinLines is the smallest number of lines a reported block spans in
	// each copy. Default 10.
	MinLines int
}

func (o Options) withDefaults() Options {
	if o.MinUnits <= 0 {
		o.MinUnits = 15
	}
	if o.MinLines <= 0 {
		o.MinLines = 10
	}
	return o
}

// Range is a span of lines in one file.
type Range struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Lines is the number of lines in the range.
func (r Range) Lines() int { return r.EndLine - r.StartLine + 1 }

// Block is a run of code found in two places. Second comes after First
// (later in the same file, or in a later file of the input).
type Block struct {
	First  Range `json:"first"`
	Second Range `json:"second"`
	Units  int   `json:"units"`
}

// Source is a parsed script to compare.
type Source struct {
	Path string
	Tree *syntax.File
}

type unit struct {
	text      string
	hash      uint64
	line, end int
}

type location struct {
	file, index int
}

// Find returns every duplicated block among sources, sorted by file and
// line. A block found in n places is reported once per pair of copies.
func Find(sources []Source, opts Options) []Block {
	opts = opts.withDefaults()
	units := make([][]unit, len(sources))
	for i, src := range sources {
		units[i] = tokenize(src.Tree)
	}

	windows := map[uint64][]location{}
	for f, us := range units {
		for i := 0; i+opts.MinUnits <= len(us); i++ {
			h := windowHash(us[i : i+opts.MinUnits])
			windows[h] = append(windows[h], location{f, i})
		}
	}

	// Every pair of equal windows, earlier location first; copies that
	// overlap in one file (a run of identical lines) are not duplicates.
	type pair struct{ a, b location }
	matched := map[pair]bool{}
	for _, locs := range windows {
		for i := 0; i < len(locs); i++ {
			for j := i + 1; j < len(locs); j++ {
				a, b := locs[i], locs[j]
				if a.file == b.file && b.index < a.index+opts.MinUnits {
					continue
				}
				if equal(units[a.file], a.index, units[b.file], b.index, opts.MinUnits) {
					matched[pair{a, b}] = true
				}
			}
		}
	}

	var blocks []Block
	for p := range matched {
		// A run is reported once, from its first window.
		if matched[pair{location{p.a.file, p.a.index - 1}, location{p.b.file, p.b.index - 1}}] {
			continue
		}
		n := extend(units, p.a, p.b, opts.MinUnits)
		block := Block{
			First:  span(sources[p.a.file].Path, units[p.a.file][p.a.index:p.a.index+n]),
			Second: span(sources[p.b.file].Path, units[p.b.file][p.b.index:p.b.index+n]),
			Units:  n,
		}
		if block.First.Lines() >= opts.MinLines && block.Second.Lines() >= opts.MinLines {
			blocks = append(blocks, block)
		}
	}
	sort.Slice(blocks, func(i, j int) bool {
		x, y := blocks[i], blocks[j]
		if x.Second.Path != y.Second.Path {
			return x.Second.Path < y.Second.Path
		}
		if x.Second.StartLine != y.Second.StartLine {
			return x.Second.StartLine < y.Second.StartLine
		}
		if x.First.Path != y.First.Path {
			return x.First.Path < y.First.Path
		}
		return x.First.StartLine < y.First.StartLine
	})
	return blocks
}

// DuplicatedLines returns, per file, the number of distinct lines that are
// part of a duplicated block (either copy).
func DuplicatedLines(blocks []Block) map[string]int {
	lines := map[string]map[int]bool{}
	mark := func(r Range) {
		if lines[r.Path] == nil {
			lines[r.Path] = map[int]bool{}
		}
		for l := r.StartLine; l <= r.EndLine; l++ {
			lines[r.Path][l] = true
		}
	}
	for _, b := range blocks {
		mark(b.First)
		mark(b.Second)
	}
	counts := map[string]int{}
	for path, set := range lines {
		counts[path] = len(set)
	}
	return counts
}

// extend grows a match of min units as far as both sides stay equal, without
// letting a copy overlap the other in the same file.
func extend(units [][]unit, a, b location, min int) int {
	ua, ub := units[a.file], units[b.file]
	n := min
	for a.index+n < len(ua) && b.index+n < len(ub) && ua[a.index+n].text == ub[b.index+n].text {
		if a.file == b.file && a.index+n >= b.index {
			break
		}
		n++
	}
	return n
}

func equal(a []unit, i int, b []unit, j, n int) bool {
	if i < 0 || j < 0 || i+n > len(a) || j+n > len(b) {
		return false
	}
	for k := 0; k < n; k++ {
		if a[i+k].text != b[j+k].text {
			return false
		}
	}
	return true
}

func span(path string, us []unit) Range {
	r := Range{Path: path, StartLine: us[0].line, EndLine: us[0].end}
	for _, u := range us {
		if u.end > r.EndLine {
			r.EndLine = u.end
		}
	}
	return r
}

func windowHash(us []unit) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, u := range us {
		for i := range buf {
			buf[i] = byte(u.hash >> (8 * i))
		}
		h.Write(buf[:])
	}
	return h.Sum64()
}

// tokenize turns a script into its units, in source order.
func tokenize(tree *syntax.File) []unit {
	t := &tokenizer{printer: syntax.NewPrinter(syntax.Minify(false))}
	t.stmts(tree.Stmts)
	return t.units
}

type tokenizer struct {
	printer *syntax.Printer
	units   []unit
}

func (t *tokenizer) keyword(text string, pos syntax.Pos) {
	t.emit(text, int(pos.Line()), int(pos.Line()))
}

func (t *tokenizer) emit(text string, line, end int) {
	h := fnv.New64a()
	h.Write([]byte(text))
	t.units = append(t.units, unit{text: text, hash: h.Sum64(), line: line, end: end})
}

func (t *tokenizer) stmts(stmts []*syntax.Stmt) {
	for _, s := range stmts {
		t.stmt(s)
	}
}

func (t *tokenizer) stmt(s *syntax.Stmt) {
	switch c := s.Cmd.(type) {
	case *syntax.IfClause:
		t.keyword("if", c.Position)
		t.stmts(c.Cond)
		t.keyword("then", c.ThenPos)
		t.stmts(c.Then)
		for e := c.Else; e != nil; e = e.Else {
			if e.ThenPos.IsValid() {
				t.keyword("elif", e.Position)
				t.stmts(e.Cond)
			} else {
				t.keyword("else", e.Position)
			}
			t.stmts(e.Then)
		}
		t.keyword("fi", c.FiPos)
	case *syntax.WhileClause:
		t.keyword(map[bool]string{false: "while", true: "until"}[c.Until], c.WhilePos)
		t.stmts(c.Cond)
		t.keyword("do", c.DoPos)
		t.stmts(c.Do)
		t.keyword("done", c.DonePos)
	case *syntax.ForClause:
		t.emit(t.print(&syntax.ForClause{Select: c.Select, Loop: c.Loop}), int(c.ForPos.Line()), int(c.DoPos.Line()))
		t.stmts(c.Do)
		t.keyword("done", c.DonePos)
	case *syntax.CaseClause:
		t.emit("case "+t.print(c.Word)+" in", int(c.Case.Line()), int(c.Case.Line()))
		for _, item := range c.Items {
			var pats []string
			for _, p := range item.Patterns {
				pats = append(pats, t.print(p))
			}
			t.keyword(strings.Join(pats, "|")+")", item.Pos())
			t.stmts(item.Stmts)
		}
		t.keyword("esac", c.Esac)
	case *syntax.Block:
		t.stmts(c.Stmts)
	case *syntax.Subshell:
		t.stmts(c.Stmts)
	case *syntax.BinaryCmd:
		t.stmt(c.X)
		t.keyword(c.Op.String(), c.OpPos)
		t.stmt(c.Y)
	case *syntax.FuncDecl:
		t.keyword("function()", c.Pos())
		t.stmt(c.Body)
	default:
		bare := *s
		bare.Comments = nil
		t.emit(t.print(&bare), int(s.Pos().Line()), int(s.End().Line()))
	}
}

func (t *tokenizer) print(node syntax.Node) string {
	var buf bytes.Buffer
	if err := t.printer.Print(&buf, node); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}
