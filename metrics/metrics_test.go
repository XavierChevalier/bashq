package metrics

import (
	"strings"
	"testing"
)

func analyze(t *testing.T, src string) *File {
	t.Helper()
	f, err := Analyze("test.sh", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func function(t *testing.T, src, name string) *Function {
	t.Helper()
	fn := analyze(t, src).Function(name)
	if fn == nil {
		t.Fatalf("no function %q", name)
	}
	return fn
}

func kinds(fn *Function) string {
	var parts []string
	for _, inc := range fn.Increments {
		parts = append(parts, inc.String())
	}
	return strings.Join(parts, ", ")
}

func TestCognitiveComplexity(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"straight code is free", `echo a; echo b | grep a`, 0},
		{"if", `if a; then b; fi`, 1},
		{"if else", `if a; then b; else c; fi`, 2},
		{"if elif else", `if a; then b; elif c; then d; else e; fi`, 3},
		{"nested if pays its depth", `if a; then if b; then c; fi; fi`, 3},
		{"three levels", `for x in y; do while a; do if b; then c; fi; done; done`, 6},
		{"else does not pay nesting", `if a; then if b; then c; else d; fi; fi`, 4},
		{"case counts once", "case $x in a) b ;; c) d ;; *) e ;; esac", 1},
		{"until", `until a; do b; done`, 1},
		{"select", `select x in a b; do c; done`, 1},
		{"c-style for", `for ((i = 0; i < 3; i++)); do a; done`, 1},
		{"sequence of like operators costs 1", `a && b && c`, 1},
		{"each change of operator costs 1", `a && b || c && d`, 3},
		{"command || return is free", `cd "$dir" || return 1`, 0},
		{"command || exit with a message is free", `cd "$dir" || { echo no >&2; exit 1; }`, 0},
		{"test && command is a hidden if", `[[ -n $x ]] && echo "$x"`, 1},
		{"test || return is a hidden if", `[[ -n $x ]] || return 1`, 1},
		{"[ ] is a test too", `[ -n "$x" ] && echo "$x"`, 1},
		{"(( )) is a test too", `(( x > 1 )) && echo big`, 1},
		{"hidden if pays nesting", `if a; then [[ -n $x ]] && echo "$x"; fi`, 3},
		{"commands after a hidden if are nested", `[[ -n $x ]] && { if a; then b; fi; }`, 3},
		{"test in an if is the if's condition", `if [[ -n $x ]] && [[ -n $y ]]; then a; fi`, 2},
		{"logical operators in [[ ]]", `if [[ a && b || c ]]; then d; fi`, 3},
		{"parentheses do not break a sequence", `if [[ a && ( b && c ) ]]; then d; fi`, 2},
		{"arithmetic operators", `(( a && b || c ))`, 2},
		{"ternary nests", `if a; then echo $(( x ? 1 : 2 )); fi`, 3},
		{"command substitution is scored where it is", `if a; then x=$(if b; then c; fi); fi`, 3},
		{"break 2 jumps two loops", `for a in b; do for c in d; do break 2; done; done`, 4},
		{"break alone is free", `for a in b; do break; done`, 1},
		{"recursion costs 1", `f`, 1},
		{"heredocs are text", "cat <<EOF\nif a; then b; fi\nEOF", 0},
		{"quotes are text", `echo "if a; then b; fi" 'while x; do y; done'`, 0},
		{"embedded awk is text", "awk '\n  { if ($1 > 2) { print } }\n'", 0},
		{"comments are text", "# if a; then b; fi\necho a", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := function(t, "f() {\n"+tc.body+"\n}\n", "f")
			if fn.Cognitive != tc.want {
				t.Errorf("cognitive = %d, want %d (%s)", fn.Cognitive, tc.want, kinds(fn))
			}
		})
	}
}

func TestIncrementsExplainTheScore(t *testing.T) {
	src := `f() {
  for x in "$@"; do
    if [[ -z $x ]]; then
      continue
    elif [[ $x == -* ]] && [[ $x != -- ]]; then
      echo opt
    fi
  done
}
`
	fn := function(t, src, "f")
	want := "+1 for, +2 if (nesting 1), +1 elif, +1 &&"
	if got := kinds(fn); got != want {
		t.Errorf("increments = %s, want %s", got, want)
	}
	if fn.Increments[1].Line != 3 || fn.Increments[1].Col != 5 {
		t.Errorf("if at %d:%d, want 3:5", fn.Increments[1].Line, fn.Increments[1].Col)
	}
	sum := 0
	for _, inc := range fn.Increments {
		sum += inc.Cost
	}
	if sum != fn.Cognitive {
		t.Errorf("increments sum to %d, cognitive is %d", sum, fn.Cognitive)
	}
}

func TestCyclomaticComplexity(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"one path", `echo a`, 1},
		{"if", `if a; then b; fi`, 2},
		{"if elif else", `if a; then b; elif c; then d; else e; fi`, 3},
		{"loop", `while a; do b; done`, 2},
		{"case counts its branches, not the catch-all", "case $x in a) ;; b|c) ;; *) ;; esac", 3},
		{"each operator is a decision", `a && b || c`, 3},
		{"guard clauses are decisions too", `cd x || return 1`, 2},
		{"operators in tests", `[[ a && b ]]`, 2},
		{"ternary", `echo $(( a ? 1 : 2 ))`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := function(t, "f() {\n"+tc.body+"\n}\n", "f")
			if fn.Cyclomatic != tc.want {
				t.Errorf("cyclomatic = %d, want %d", fn.Cyclomatic, tc.want)
			}
		})
	}
}

func TestMaxNesting(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{`echo a`, 0},
		{`if a; then b; fi`, 1},
		{`if a; then b; elif c; then if d; then e; fi; fi`, 2},
		{`for a in b; do case $a in x) while c; do d; done ;; esac; done`, 3},
		{`[[ -n $x ]] && { if a; then b; fi; }`, 2},
	}
	for _, tc := range cases {
		fn := function(t, "f() {\n"+tc.body+"\n}\n", "f")
		if fn.MaxNesting != tc.want {
			t.Errorf("%q: max nesting = %d, want %d", tc.body, fn.MaxNesting, tc.want)
		}
	}
}

func TestFunctions(t *testing.T) {
	src := `#!/usr/bin/env bash
# A library.

one() { echo 1; }

function two {
  if a; then
    inner() {
      if b; then c; fi
    }
  fi
}

ns::three() (
  echo 3
)

if main; then exit; fi
`
	f := analyze(t, src)
	var names []string
	for _, fn := range f.Functions {
		names = append(names, fn.Name)
	}
	if got := strings.Join(names, " "); got != "one two inner ns::three" {
		t.Fatalf("functions = %s", got)
	}

	two := f.Function("two")
	if two.Line != 6 || two.EndLine != 12 || two.Lines != 7 {
		t.Errorf("two spans %d-%d (%d lines), want 6-12 (7 lines)", two.Line, two.EndLine, two.Lines)
	}
	if two.Cognitive != 1 {
		t.Errorf("two = %d: a nested function must not count in its parent", two.Cognitive)
	}
	if inner := f.Function("inner"); inner.Cognitive != 1 {
		t.Errorf("inner = %d, want 1: a function's nesting starts at 0", inner.Cognitive)
	}
	if f.TopLevel.Cognitive != 1 {
		t.Errorf("top level = %d, want 1", f.TopLevel.Cognitive)
	}
	if f.Cognitive != 3 {
		t.Errorf("file = %d, want 3 (sum of functions and top level)", f.Cognitive)
	}
	if got := f.FunctionAt(9).Name; got != "inner" {
		t.Errorf("line 9 is in %s, want inner", got)
	}
	if got := f.FunctionAt(18).Name; got != TopLevel {
		t.Errorf("line 18 is in %s, want the top level", got)
	}
}

func TestLineCounts(t *testing.T) {
	src := "#!/usr/bin/env bash\n\n# comment\necho a # trailing\ncat <<EOF\n# not a comment\nEOF\n"
	f := analyze(t, src)
	if f.Lines != 7 || f.CodeLines != 4 || f.CommentLines != 3 {
		t.Errorf("lines=%d code=%d comments=%d, want 7 4 3", f.Lines, f.CodeLines, f.CommentLines)
	}
}

func TestUnboundedLoops(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"while true", `while true; do a; done`, 1},
		{"while :", `while :; do a; done`, 1},
		{"until false", `until false; do a; done`, 1},
		{"while (( 1 ))", `while (( 1 )); do a; done`, 1},
		{"while ! false", `while ! false; do a; done`, 1},
		{"until ! true", `until ! true; do a; done`, 1},
		{"in a pipeline's command substitution", `x=$(while true; do a; done)`, 1},
		{"nested loops count each", `while true; do while :; do a; done; done`, 2},
		{"a command may fail", `while read -r l; do a; done`, 0},
		{"a test is a bound", `while [[ $i -lt 3 ]]; do a; done`, 0},
		{"while false never runs", `while false; do a; done`, 0},
		{"until true never runs", `until true; do a; done`, 0},
		{"while (( 0 )) never runs", `while (( 0 )); do a; done`, 0},
		{"while (( n )) is a variable", `while (( n )); do a; done`, 0},
		{"true with an argument", `while true x; do a; done`, 0},
		{"two commands: the last one decides", `while a; true; do b; done`, 0},
		{"redirected", `while true >/dev/null; do a; done`, 0},
		{"assignment", `while x=1 true; do a; done`, 0},
		{"quoted", `while 'true'; do a; done`, 0},
		{"a string", `echo "while true; do x; done"`, 0},
		{"a heredoc", "cat <<EOF\nwhile true; do x; done\nEOF", 0},
		{"a comment", "# while true; do x; done\necho", 0},
		{"for is bounded by its list", `for x in true; do a; done`, 0},
	}
	for _, tc := range cases {
		fn := function(t, "f() {\n"+tc.body+"\n}\n", "f")
		if got := len(fn.UnboundedLoops); got != tc.want {
			t.Errorf("%s: %q: %d unbounded loops, want %d", tc.name, tc.body, got, tc.want)
		}
	}

	fn := function(t, "f() {\n  while :; do a; done\n}\n", "f")
	if got := fn.UnboundedLoops[0]; got != (Position{Line: 2, Col: 3}) {
		t.Errorf("position = %+v, want line 2 col 3 (the while keyword)", got)
	}
	if top := analyze(t, "f() { :; }\nwhile true; do a; done\n").TopLevel; len(top.UnboundedLoops) != 1 {
		t.Errorf("a loop outside functions is the top level's")
	}
}

func TestLineWidths(t *testing.T) {
	src := "echo a\n\techo b\nécho\r\ncat <<EOF\n" + strings.Repeat("x", 120) + "\nEOF\n"
	f := analyze(t, src)
	want := []int{6, 7, 4, 9, 120, 3}
	if len(f.LineWidths) != len(want) {
		t.Fatalf("widths = %v, want %v", f.LineWidths, want)
	}
	for i := range want {
		if f.LineWidths[i] != want[i] {
			t.Errorf("widths = %v, want %v: runes, a tab counts 1, no \\r, heredocs count", f.LineWidths, want)
			break
		}
	}
}

func TestParseError(t *testing.T) {
	_, err := Analyze("bad.sh", []byte("if a; then\n"))
	if err == nil || !strings.Contains(err.Error(), "bad.sh:") {
		t.Fatalf("err = %v, want a positioned parse error", err)
	}
}

func FuzzAnalyze(f *testing.F) {
	for _, seed := range []string{
		"f() { if a; then b; fi; }",
		"[[ -n $x ]] && y || z",
		"echo $(( a ? b : c ))",
		"case $x in a) ;; esac",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		file, err := Analyze("fuzz.sh", []byte(src))
		if err != nil {
			return
		}
		for _, fn := range append(file.Functions, file.TopLevel) {
			sum := 0
			for _, inc := range fn.Increments {
				sum += inc.Cost
			}
			if sum != fn.Cognitive || fn.Cyclomatic < 1 || fn.Cognitive < 0 {
				t.Fatalf("%s: cognitive %d, increments %d, cyclomatic %d", fn.Name, fn.Cognitive, sum, fn.Cyclomatic)
			}
		}
	})
}
