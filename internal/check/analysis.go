package check

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/syntax"

	"github.com/XavierChevalier/bashq/dup"
	"github.com/XavierChevalier/bashq/metrics"
)

// Analysis holds the measures of a set of scripts.
type Analysis struct {
	// Files are the measured scripts, sorted by path.
	Files []*metrics.File
	// Errors are the scripts that could not be read or parsed.
	Errors []FileError
	// Blocks are the duplicated blocks among Files.
	Blocks []dup.Block
	// DuplicatedLines counts, per path, the lines in a duplicated block.
	DuplicatedLines map[string]int
}

// FileError is a script that could not be read or parsed.
type FileError struct {
	Path string
	Line int
	Col  int
	Err  error
}

// Analyze reads, parses and measures paths in parallel, then looks for
// duplicated blocks among them (unless dupOpts is nil).
func Analyze(paths []string, dupOpts *dup.Options) *Analysis {
	type parsed struct {
		file *metrics.File
		tree *syntax.File
		err  *FileError
	}
	results := make([]parsed, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range jobs {
				path := paths[i]
				src, err := os.ReadFile(path)
				if err != nil {
					results[i].err = &FileError{Path: path, Err: err}
					continue
				}
				tree, err := metrics.Parse(path, src)
				if err != nil {
					results[i].err = parseError(path, err)
					continue
				}
				results[i] = parsed{file: metrics.Measure(path, src, tree), tree: tree}
			}
		})
	}
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	a := &Analysis{}
	var sources []dup.Source
	for _, r := range results {
		if r.err != nil {
			a.Errors = append(a.Errors, *r.err)
			continue
		}
		a.Files = append(a.Files, r.file)
		sources = append(sources, dup.Source{Path: r.file.Path, Tree: r.tree})
	}
	if dupOpts != nil {
		a.Blocks = dup.Find(sources, *dupOpts)
	}
	a.DuplicatedLines = dup.DuplicatedLines(a.Blocks)
	return a
}

func parseError(path string, err error) *FileError {
	fe := &FileError{Path: path, Err: err}
	var pe syntax.ParseError
	if errors.As(err, &pe) {
		fe.Line, fe.Col, fe.Err = int(pe.Pos.Line()), int(pe.Pos.Col()), errors.New(pe.Text)
	}
	var le syntax.LangError
	if errors.As(err, &le) {
		// Its message starts with the position, which the finding has.
		msg := le.Error()
		if _, rest, ok := strings.Cut(msg, fmt.Sprintf("%d:%d: ", le.Pos.Line(), le.Pos.Col())); ok {
			msg = rest
		}
		fe.Line, fe.Col, fe.Err = int(le.Pos.Line()), int(le.Pos.Col()), errors.New(msg)
	}
	return fe
}

// File returns the measures of path, or nil.
func (a *Analysis) File(path string) *metrics.File {
	i := sort.Search(len(a.Files), func(i int) bool { return a.Files[i].Path >= path })
	if i < len(a.Files) && a.Files[i].Path == path {
		return a.Files[i]
	}
	return nil
}

// Location is a place in a script.
type Location struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Col     int    `json:"col,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
}

// Finding is one problem reported by a rule.
type Finding struct {
	// Rule is the rule name; for ShellCheck, the code (SC2086).
	Rule string `json:"rule"`
	Location
	// Function is the function the finding is in (metrics.TopLevel outside
	// functions, "" for a whole file). The baseline keys on it rather than
	// on lines, so it survives edits elsewhere in the file.
	Function string `json:"function,omitempty"`
	// Value is the measure (Measure rules, and the width of a line for
	// line-length); 1 for other Count rules.
	Value int `json:"value"`
	// Max is the threshold Value went over (rules with a max).
	Max      int    `json:"max,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Related is the other copy of a duplicated block.
	Related *Location `json:"related,omitempty"`
	// Allowed is the value or count the baseline tolerates, when the
	// baseline has a row for this finding that it exceeds.
	Allowed int `json:"allowed,omitempty"`
}

// Findings applies the rules to an analysis. ShellCheck findings are added
// separately (see ShellCheckFindings).
func (a *Analysis) Findings(s *Settings) []Finding {
	var out []Finding
	add := func(f Finding) {
		if !s.Enabled(f.Rule) {
			return
		}
		r, _ := Lookup(f.Rule)
		if f.Severity == "" {
			f.Severity = r.Severity
		}
		if f.Value == 0 {
			f.Value = 1
		}
		out = append(out, f)
	}
	over := func(rule string, value int) bool {
		return s.Enabled(rule) && value > s.Max(rule)
	}

	for _, e := range a.Errors {
		add(Finding{Rule: "parse-error", Location: Location{Path: e.Path, Line: max(e.Line, 1), Col: e.Col}, Message: e.Err.Error()})
	}
	for _, file := range a.Files {
		if over("file-length", file.Lines) {
			add(Finding{
				Rule: "file-length", Location: Location{Path: file.Path, Line: 1},
				Value: file.Lines, Max: s.Max("file-length"),
				Message: fmt.Sprintf("file is %d lines long (max %d)", file.Lines, s.Max("file-length")),
			})
		}
		for _, fn := range file.Functions {
			loc := Location{Path: file.Path, Line: fn.Line, EndLine: fn.EndLine}
			if over("cognitive-complexity", fn.Cognitive) {
				add(Finding{
					Rule: "cognitive-complexity", Location: loc, Function: fn.Name,
					Value: fn.Cognitive, Max: s.Max("cognitive-complexity"),
					Message: fmt.Sprintf("%s has a cognitive complexity of %d (max %d)", fn.Name, fn.Cognitive, s.Max("cognitive-complexity")),
				})
			}
			if over("cyclomatic-complexity", fn.Cyclomatic) {
				add(Finding{
					Rule: "cyclomatic-complexity", Location: loc, Function: fn.Name,
					Value: fn.Cyclomatic, Max: s.Max("cyclomatic-complexity"),
					Message: fmt.Sprintf("%s has a cyclomatic complexity of %d (max %d)", fn.Name, fn.Cyclomatic, s.Max("cyclomatic-complexity")),
				})
			}
			if over("function-length", fn.Lines) {
				add(Finding{
					Rule: "function-length", Location: loc, Function: fn.Name,
					Value: fn.Lines, Max: s.Max("function-length"),
					Message: fmt.Sprintf("%s is %d lines long (max %d)", fn.Name, fn.Lines, s.Max("function-length")),
				})
			}
		}
		if s.Enabled("line-length") {
			for i, width := range file.LineWidths {
				if over("line-length", width) {
					add(Finding{
						Rule: "line-length", Location: Location{Path: file.Path, Line: i + 1, Col: s.Max("line-length") + 1},
						Function: file.FunctionAt(i + 1).Name, Value: width, Max: s.Max("line-length"),
						Message: fmt.Sprintf("line is %d characters long (max %d)", width, s.Max("line-length")),
					})
				}
			}
		}
		for _, fn := range append([]*metrics.Function{file.TopLevel}, file.Functions...) {
			for _, loop := range fn.UnboundedLoops {
				add(Finding{
					Rule: "unbounded-loop", Location: Location{Path: file.Path, Line: loop.Line, Col: loop.Col}, Function: fn.Name,
					Message: "loop with no upper bound: bound it (a max count or a deadline) or assert one inside",
				})
			}
			if over("nesting-depth", fn.MaxNesting) {
				add(Finding{
					Rule: "nesting-depth", Location: Location{Path: file.Path, Line: fn.MaxNestingLine}, Function: fn.Name,
					Value: fn.MaxNesting, Max: s.Max("nesting-depth"),
					Message: fmt.Sprintf("%s nests control structures %d deep (max %d)", fn.Name, fn.MaxNesting, s.Max("nesting-depth")),
				})
			}
		}
	}
	for _, b := range a.Blocks {
		fn := ""
		if file := a.File(b.Second.Path); file != nil {
			fn = file.FunctionAt(b.Second.StartLine).Name
		}
		add(Finding{
			Rule:     "duplication",
			Location: Location{Path: b.Second.Path, Line: b.Second.StartLine, EndLine: b.Second.EndLine},
			Function: fn,
			Message: fmt.Sprintf("%d lines duplicate %s:%d-%d",
				b.Second.Lines(), b.First.Path, b.First.StartLine, b.First.EndLine),
			Related: &Location{Path: b.First.Path, Line: b.First.StartLine, EndLine: b.First.EndLine},
		})
	}
	Sort(out)
	return out
}

// Sort orders findings by path, line, then rule.
func Sort(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		x, y := findings[i], findings[j]
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Rule < y.Rule
	})
}
