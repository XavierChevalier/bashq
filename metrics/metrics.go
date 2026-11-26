// Package metrics measures bash and POSIX sh source code: cognitive and
// cyclomatic complexity, length and nesting depth of every function, and the
// size of every file.
//
// The measures are computed on the syntax tree of mvdan.cc/sh (the parser of
// shfmt), never on the text, so quotes, heredocs, comments and embedded awk
// or jq programs cannot fool them. How each one is counted is documented in
// docs/metrics.md.
package metrics

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

// TopLevel is the name of the pseudo-function that holds the code of a file
// outside any function.
const TopLevel = "(top level)"

// File holds the measures of one script.
type File struct {
	Path string `json:"path"`
	// Lines is the number of physical lines.
	Lines int `json:"lines"`
	// CodeLines is the number of lines holding code: neither blank nor a
	// comment alone (Sonar's ncloc).
	CodeLines int `json:"code_lines"`
	// CommentLines is the number of lines holding a comment.
	CommentLines int `json:"comment_lines"`
	// Cognitive is the sum of the cognitive complexity of the functions and
	// the top-level code.
	Cognitive int `json:"cognitive"`
	// Cyclomatic is the sum of the cyclomatic complexity of the functions and
	// the top-level code.
	Cyclomatic int `json:"cyclomatic"`
	// Functions lists the functions in source order. Nested functions are
	// listed on their own and do not count in the function around them.
	Functions []*Function `json:"functions"`
	// TopLevel measures the code outside any function.
	TopLevel *Function `json:"top_level"`
	// LineWidths is the width of each physical line, in characters (runes;
	// a tab counts 1). Unlike the other measures it is read on the text:
	// heredoc bodies, strings and comments count.
	LineWidths []int `json:"-"`
}

// Function holds the measures of one function.
type Function struct {
	Name    string `json:"name"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
	// Lines is the number of lines from the name to the closing brace.
	Lines int `json:"lines"`
	// Cognitive is the cognitive complexity (SonarSource), adapted to bash.
	Cognitive int `json:"cognitive"`
	// Cyclomatic is McCabe's cyclomatic complexity: 1 + decision points.
	Cyclomatic int `json:"cyclomatic"`
	// MaxNesting is the deepest nesting of control structures (if, case,
	// loops, conditional lists) in the function.
	MaxNesting int `json:"max_nesting"`
	// MaxNestingLine is the line of the structure that reaches MaxNesting.
	MaxNestingLine int `json:"max_nesting_line"`
	// Increments explains Cognitive: each structure that costs something.
	Increments []Increment `json:"increments"`
	// UnboundedLoops are the while and until loops of the function whose
	// condition is a constant that never ends them (while true, while :,
	// until false, while (( 1 ))): only a break or a return gets out.
	UnboundedLoops []Position `json:"-"`
}

// Position is a place in a script.
type Position struct {
	Line int
	Col  int
}

// Increment is one contribution to a cognitive complexity.
type Increment struct {
	Line int `json:"line"`
	Col  int `json:"col"`
	// Kind is the construct: "if", "elif", "else", "for", "while", "until",
	// "select", "case", "&&", "||", "?:", "conditional", "recursion",
	// "break N", "continue N".
	Kind string `json:"kind"`
	// Nesting is the nesting level the construct was found at; it is part of
	// Cost for structures that can nest.
	Nesting int `json:"nesting"`
	Cost    int `json:"cost"`
}

// String renders an increment the way `bashq explain` shows it.
func (i Increment) String() string {
	if i.Nesting > 0 && i.Cost > 1 {
		return fmt.Sprintf("+%d %s (nesting %d)", i.Cost, i.Kind, i.Nesting)
	}
	return fmt.Sprintf("+%d %s", i.Cost, i.Kind)
}

// Function returns the function named name, or nil. TopLevel names the code
// outside functions.
func (f *File) Function(name string) *Function {
	if name == TopLevel {
		return f.TopLevel
	}
	for _, fn := range f.Functions {
		if fn.Name == name {
			return fn
		}
	}
	return nil
}

// FunctionAt returns the innermost function holding line, or the top level.
func (f *File) FunctionAt(line int) *Function {
	var found *Function
	for _, fn := range f.Functions {
		if fn.Line <= line && line <= fn.EndLine {
			if found == nil || fn.Line >= found.Line {
				found = fn
			}
		}
	}
	if found == nil {
		return f.TopLevel
	}
	return found
}

// Parse parses a bash script. Every bashq measure starts from its result.
func Parse(path string, src []byte) (*syntax.File, error) {
	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(syntax.LangBash))
	return parser.Parse(bytes.NewReader(src), path)
}

// Analyze parses src and measures it.
func Analyze(path string, src []byte) (*File, error) {
	tree, err := Parse(path, src)
	if err != nil {
		return nil, err
	}
	return Measure(path, src, tree), nil
}

// Measure measures an already parsed script; src is the text it was parsed
// from, for the line counts.
func Measure(path string, src []byte, tree *syntax.File) *File {
	file := &File{Path: path}
	countLines(file, src, tree)

	top := &Function{Name: TopLevel, Line: 1, EndLine: file.Lines, Lines: file.Lines}
	w := &walker{file: file, fn: top}
	w.stmts(tree.Stmts, 0)
	file.TopLevel = finish(top)
	for _, fn := range file.Functions {
		file.Cognitive += fn.Cognitive
		file.Cyclomatic += fn.Cyclomatic
	}
	file.Cognitive += top.Cognitive
	file.Cyclomatic += top.Cyclomatic
	return file
}

func finish(fn *Function) *Function {
	fn.Cyclomatic++
	if fn.Increments == nil {
		fn.Increments = []Increment{}
	}
	return fn
}

// countLines fills Lines, CodeLines and CommentLines. A line is a comment
// line when a comment of the tree starts on it; it is a code line when it
// holds something else than blanks and that comment.
func countLines(file *File, src []byte, tree *syntax.File) {
	text := string(src)
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	file.Lines = len(lines)
	file.LineWidths = make([]int, len(lines))
	for i, line := range lines {
		file.LineWidths[i] = utf8.RuneCountInString(strings.TrimSuffix(line, "\r"))
	}

	commentCol := map[int]int{}
	syntax.Walk(tree, func(node syntax.Node) bool {
		if c, ok := node.(*syntax.Comment); ok {
			commentCol[int(c.Hash.Line())] = int(c.Hash.Col())
		}
		return true
	})
	// Comments attached to the last statement of a file are not always
	// reached by Walk; tree.Last holds them.
	for _, c := range tree.Last {
		commentCol[int(c.Hash.Line())] = int(c.Hash.Col())
	}

	for i, line := range lines {
		col, isComment := commentCol[i+1]
		code := line
		if isComment {
			file.CommentLines++
			if col-1 <= len(code) {
				code = code[:col-1]
			}
		}
		if strings.TrimSpace(code) != "" {
			file.CodeLines++
		}
	}
}
