package check

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XavierChevalier/bashq/internal/config"
)

// complex is a function of cognitive complexity 6.
const complex = `f() {
  for a in "$@"; do
    if [[ -n $a ]]; then
      while b; do c; done
    fi
  done
}
`

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func cfgWith(t *testing.T, yaml string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func rules(findings []Finding) string {
	var parts []string
	for _, f := range findings {
		parts = append(parts, f.Rule+"@"+f.Path+":"+f.Function)
	}
	return strings.Join(parts, " ")
}

func TestThresholds(t *testing.T) {
	project(t, map[string]string{"a.sh": complex})
	cfg := cfgWith(t, "rules:\n  cognitive-complexity: {max: 5}\nshellcheck: {enabled: never}\n")
	r, err := Run(Options{Config: cfg, Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(r.New); got != "cognitive-complexity@a.sh:f" {
		t.Fatalf("findings = %s", got)
	}
	f := r.New[0]
	if f.Value != 6 || f.Max != 5 || f.Line != 1 || f.Message != "f has a cognitive complexity of 6 (max 5)" {
		t.Errorf("finding = %+v", f)
	}

	cfg = cfgWith(t, "rules:\n  cognitive-complexity: {max: 6}\nshellcheck: {enabled: never}\n")
	if r, _ := Run(Options{Config: cfg, Paths: []string{"."}}); len(r.New) != 0 {
		t.Errorf("at the max: findings = %s, want none", rules(r.New))
	}
}

func TestRuleSettings(t *testing.T) {
	cfg := cfgWith(t, "rules:\n  cyclomatic-complexity: {enabled: true, max: 3}\n  function-length: {enabled: false}\n")
	s, err := Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled("cyclomatic-complexity") || s.Max("cyclomatic-complexity") != 3 || s.Enabled("function-length") {
		t.Errorf("settings not applied")
	}
	if !s.Enabled("SC2086") {
		t.Errorf("ShellCheck codes follow the shellcheck rule")
	}

	for _, bad := range []string{
		"rules:\n  cognitive: {max: 3}\n",
		"rules:\n  duplication: {max: 3}\n",
		"rules:\n  nesting-depth: {max: -1}\n",
	} {
		if _, err := Resolve(cfgWith(t, bad)); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	if err := s.Only([]string{"function-length", "SC2086"}); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled("function-length") || s.Enabled("cognitive-complexity") || !s.Enabled("parse-error") || !s.Enabled("SC1000") {
		t.Errorf("Only: wrong rules enabled")
	}
}

func TestParseErrorsAreFindings(t *testing.T) {
	project(t, map[string]string{"bad.sh": "f() {\n  if a; then\n}\n", "good.sh": "echo ok\n"})
	r, err := Run(Options{Config: cfgWith(t, "shellcheck: {enabled: never}\n"), Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.New) != 1 || r.New[0].Rule != "parse-error" || r.New[0].Path != "bad.sh" || r.New[0].Line == 0 {
		t.Fatalf("findings = %+v", r.New)
	}
	if len(r.Analysis.Files) != 1 {
		t.Errorf("the good file must still be measured")
	}
}

func TestDuplicationFinding(t *testing.T) {
	var body strings.Builder
	for i := range 12 {
		body.WriteString("  step_" + string(rune('a'+i)) + " --flag\n")
	}
	project(t, map[string]string{
		"a.sh": "a() {\n" + body.String() + "}\n",
		"b.sh": "b() {\n" + body.String() + "}\n",
	})
	cfg := cfgWith(t, "duplication: {min-units: 10, min-lines: 10}\nshellcheck: {enabled: never}\n")
	r, err := Run(Options{Config: cfg, Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(r.New); got != "duplication@b.sh:b" {
		t.Fatalf("findings = %s", got)
	}
	if r.New[0].Related == nil || r.New[0].Related.Path != "a.sh" {
		t.Errorf("related = %+v", r.New[0].Related)
	}

	cfg = cfgWith(t, "rules:\n  duplication: {enabled: false}\nshellcheck: {enabled: never}\n")
	if r, _ := Run(Options{Config: cfg, Paths: []string{"."}}); len(r.New) != 0 || len(r.Analysis.Blocks) != 0 {
		t.Errorf("duplication disabled: %s", rules(r.New))
	}
}

func TestBaselineRatchet(t *testing.T) {
	dir := project(t, map[string]string{"a.sh": complex, "b.sh": "g() { if a; then b; fi; }\n"})
	cfg := cfgWith(t, "rules:\n  cognitive-complexity: {max: 1}\nshellcheck: {enabled: never}\n")
	baseline := filepath.Join(dir, "baseline.tsv")
	run := func() *Report {
		t.Helper()
		r, err := Run(Options{Config: cfg, Paths: []string{"."}, Baseline: baseline, Ratchet: true})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// Accept today's debt.
	var buf bytes.Buffer
	if err := NewBaseline(run().All).Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cognitive-complexity\ta.sh\tf\t6\n") {
		t.Fatalf("baseline =\n%s", buf.String())
	}
	if err := os.WriteFile(baseline, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := run(); len(r.New) != 0 || r.Baselined != 1 {
		t.Fatalf("with baseline: new = %s, baselined = %d", rules(r.New), r.Baselined)
	}

	// A new problem fails.
	os.WriteFile("b.sh", []byte("g() { if a; then if b; then c; fi; fi; }\n"), 0o644)
	if got := rules(run().New); got != "cognitive-complexity@b.sh:g" {
		t.Errorf("new problem: findings = %s", got)
	}

	// A worse function fails, and says what the baseline allowed.
	os.WriteFile("b.sh", []byte("g() { :; }\n"), 0o644)
	os.WriteFile("a.sh", []byte(strings.Replace(complex, "c; done", "if c; then d; fi; done", 1)), 0o644)
	r := run()
	if got := rules(r.New); got != "cognitive-complexity@a.sh:f" || r.New[0].Allowed != 6 || r.New[0].Value != 10 {
		t.Errorf("worse: findings = %s %+v", got, r.New)
	}

	// A better function makes its row stale: the baseline must shrink.
	os.WriteFile("a.sh", []byte("f() { if a; then if b; then c; fi; fi; }\n"), 0o644)
	r = run()
	if got := rules(r.New); got != "baseline-stale@a.sh:f" || !strings.Contains(r.New[0].Message, "at 6, the code is now at 3") {
		t.Errorf("better: findings = %s %+v", got, r.New)
	}
	if r, _ := Run(Options{Config: cfg, Paths: []string{"."}, Baseline: baseline}); len(r.New) != 0 {
		t.Errorf("without ratchet: findings = %s", rules(r.New))
	}

	// A fixed function's row must go.
	os.WriteFile("a.sh", []byte("f() { :; }\n"), 0o644)
	if r := run(); len(r.New) != 1 || !strings.Contains(r.New[0].Message, "remove the row") {
		t.Errorf("fixed: findings = %+v", r.New)
	}

	// Rows of files not analysed are left alone.
	if r, _ := Run(Options{Config: cfg, Paths: []string{"b.sh"}, Baseline: baseline, Ratchet: true}); len(r.New) != 0 {
		t.Errorf("subset: findings = %s", rules(r.New))
	}
}

func TestCountRulesInTheBaseline(t *testing.T) {
	b := Baseline{{Rule: "SC2086", Path: "a.sh", Function: "f"}: 2}
	s, _ := Resolve(config.Default())
	analysed := map[string]bool{"a.sh": true}
	f := Finding{Rule: "SC2086", Location: Location{Path: "a.sh", Line: 3}, Function: "f", Value: 1}

	if out := b.Apply([]Finding{f, f}, s, analysed, true); len(out.New) != 0 || out.Baselined != 2 {
		t.Errorf("two tolerated: %+v", out)
	}
	if out := b.Apply([]Finding{f, f, f}, s, analysed, true); len(out.New) != 3 {
		t.Errorf("three: all reported, got %+v", out)
	}
	if out := b.Apply([]Finding{f}, s, analysed, true); len(out.New) != 1 || out.New[0].Rule != "baseline-stale" {
		t.Errorf("one: the row is stale, got %+v", out)
	}
	s.Disable("shellcheck")
	if out := b.Apply(nil, s, analysed, true); len(out.New) != 0 {
		t.Errorf("shellcheck did not run: its rows are not stale, got %+v", out)
	}
}

func TestLineLength(t *testing.T) {
	wide := "  echo " + strings.Repeat("x", 30)
	project(t, map[string]string{"a.sh": "#!/usr/bin/env bash\nf() {\n" + wide + "\n" + wide + "x\n}\n" + wide + "\n"})
	cfg := cfgWith(t, "rules:\n  line-length: {max: 36}\nshellcheck: {enabled: never}\n")
	r, err := Run(Options{Config: cfg, Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(r.New); got != "line-length@a.sh:f line-length@a.sh:f line-length@a.sh:(top level)" {
		t.Fatalf("findings = %s", got)
	}
	f := r.New[1]
	if f.Line != 4 || f.Col != 37 || f.Value != 38 || f.Max != 36 || f.Message != "line is 38 characters long (max 36)" {
		t.Errorf("finding = %+v", f)
	}

	// The baseline counts the long lines of a function: one more fails,
	// even a shorter one; one fewer is stale.
	b := NewBaseline(r.New)
	if got := b[Key{Rule: "line-length", Path: "a.sh", Function: "f"}]; got != 2 {
		t.Errorf("baseline row = %d, want 2 lines", got)
	}
	s, _ := Resolve(cfg)
	analysed := map[string]bool{"a.sh": true}
	if out := b.Apply(append(r.New, r.New[0]), s, analysed, true); len(out.New) != 3 {
		t.Errorf("a third long line in f: want its 3 lines reported, got %s", rules(out.New))
	}
	if out := b.Apply(r.New[1:], s, analysed, true); rules(out.New) != "baseline-stale@a.sh:f" {
		t.Errorf("one line fixed: want a stale row, got %s", rules(out.New))
	}

	cfg = cfgWith(t, "rules:\n  line-length: {max: 38}\nshellcheck: {enabled: never}\n")
	if r, _ := Run(Options{Config: cfg, Paths: []string{"."}}); len(r.New) != 0 {
		t.Errorf("at the max: findings = %s, want none", rules(r.New))
	}
}

func TestUnboundedLoop(t *testing.T) {
	project(t, map[string]string{"a.sh": "f() {\n  while true; do\n    poll || break\n  done\n}\nwhile :; do f; done\n"})
	r, err := Run(Options{Config: cfgWith(t, "shellcheck: {enabled: never}\n"), Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(r.New); got != "unbounded-loop@a.sh:f unbounded-loop@a.sh:(top level)" {
		t.Fatalf("findings = %s", got)
	}
	f := r.New[0]
	if f.Line != 2 || f.Col != 3 || f.Value != 1 || f.Severity != "warning" ||
		f.Message != "loop with no upper bound: bound it (a max count or a deadline) or assert one inside" {
		t.Errorf("finding = %+v", f)
	}
	if _, err := Resolve(cfgWith(t, "rules:\n  unbounded-loop: {max: 3}\n")); err == nil {
		t.Errorf("unbounded-loop has no max: want an error")
	}
}

func TestReadBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.tsv")
	if b, err := ReadBaseline(path); err != nil || len(b) != 0 {
		t.Fatalf("missing file: %v %v", b, err)
	}
	os.WriteFile(path, []byte("# comment\n\nfunction-length\ta.sh\tf\t80\n"), 0o644)
	b, err := ReadBaseline(path)
	if err != nil || b[Key{"function-length", "a.sh", "f"}] != 80 {
		t.Fatalf("baseline = %v, %v", b, err)
	}
	for _, bad := range []string{"a\tb\tc\n", "r\tp\tf\tx\n", "r\tp\tf\t0\n"} {
		os.WriteFile(path, []byte(bad), 0o644)
		if _, err := ReadBaseline(path); err == nil || !strings.Contains(err.Error(), "b.tsv:1") {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestShellCheck(t *testing.T) {
	project(t, map[string]string{"a.sh": "f() {\n  echo $1\n}\n"})
	// A fake shellcheck: prints one json1 comment per file and exits 1, like
	// the real one when it finds something.
	bin := t.TempDir()
	fake := filepath.Join(bin, "shellcheck")
	script := `#!/bin/sh
for last; do :; done
printf '{"comments":[{"file":"%s","line":2,"endLine":2,"column":8,"endColumn":10,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."}]}' "$last"
exit 1
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := cfgWith(t, "shellcheck: {enabled: always, path: "+fake+"}\n")
	r, err := Run(Options{Config: cfg, Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if !r.ShellCheck || rules(r.New) != "SC2086@a.sh:f" || r.New[0].Severity != "note" || r.New[0].Col != 8 {
		t.Fatalf("findings = %+v", r.New)
	}

	cfg = cfgWith(t, "shellcheck: {enabled: always, path: /does/not/exist}\n")
	if _, err := Run(Options{Config: cfg, Paths: []string{"."}}); err != ErrNoShellCheck {
		t.Errorf("missing and required: err = %v", err)
	}
	cfg = cfgWith(t, "shellcheck: {enabled: auto, path: /does/not/exist}\n")
	if r, err := Run(Options{Config: cfg, Paths: []string{"."}}); err != nil || r.ShellCheck {
		t.Errorf("missing and auto: err = %v, ran = %v", err, r.ShellCheck)
	}

	os.WriteFile(fake, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755)
	cfg = cfgWith(t, "shellcheck: {enabled: always, path: "+fake+"}\n")
	if _, err := Run(Options{Config: cfg, Paths: []string{"."}}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("shellcheck crash: err = %v", err)
	}
}

func TestRuleExclude(t *testing.T) {
	var body strings.Builder
	for i := range 12 {
		body.WriteString("  step_" + string(rune('a'+i)) + " --flag\n")
	}
	dir := project(t, map[string]string{
		"lib/a.sh":       complex,
		"oneshot/old.sh": "a() {\n" + body.String() + "}\n",
		"oneshot/new.sh": "b() {\n" + body.String() + "}\n" + complex,
	})
	cfg := cfgWith(t, `
rules:
  cognitive-complexity: {max: 1, exclude: ["lib/*.sh"]}
  duplication: {exclude: [oneshot]}
duplication: {min-units: 10, min-lines: 10}
shellcheck: {enabled: never}
`)
	r, err := Run(Options{Config: cfg, Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules(r.New); got != "cognitive-complexity@oneshot/new.sh:f" {
		t.Errorf("findings = %s: excluded paths must drop only their rule", got)
	}

	// A baseline row on an excluded path is not stale: the rule does not apply there.
	baseline := filepath.Join(dir, "b.tsv")
	os.WriteFile(baseline, []byte("cognitive-complexity\tlib/a.sh\tf\t6\ncognitive-complexity\toneshot/new.sh\tf\t6\n"), 0o644)
	r, _ = Run(Options{Config: cfg, Paths: []string{"."}, Baseline: baseline, Ratchet: true})
	if len(r.New) != 0 {
		t.Errorf("findings = %s, want none", rules(r.New))
	}
}

func TestParseErrorMessageHasNoPosition(t *testing.T) {
	project(t, map[string]string{"zsh.sh": "f() {\n  local -A x=([a.b]=1)\n}\n"})
	r, err := Run(Options{Config: cfgWith(t, "shellcheck: {enabled: never}\n"), Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.New) != 1 || r.New[0].Line != 2 || strings.Contains(r.New[0].Message, "zsh.sh") {
		t.Errorf("findings = %+v", r.New)
	}
}
